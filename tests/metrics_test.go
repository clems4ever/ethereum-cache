package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPrometheusMetrics(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a dummy transaction with valid hash and required fields
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"hash":"0x0000000000000000000000000000000000000000000000000000000000000123", "nonce":"0x0", "blockHash":"0x0000000000000000000000000000000000000000000000000000000000000000", "blockNumber":"0x1", "transactionIndex":"0x0", "from":"0x0000000000000000000000000000000000000000", "to":"0x0000000000000000000000000000000000000000", "value":"0x0", "gas":"0x0", "gasPrice":"0x0", "input":"0x", "v":"0x0", "r":"0x0", "s":"0x0"}}`))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8090"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// Helper to get metric value
	getMetric := func(name string, method string) float64 {
		resp, err := http.Get("http://localhost:" + proxyPort + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Pattern: name{method="method"} value
		// e.g. ethereum_cache_misses_total{method="eth_getTransactionByHash"} 1
		pattern := fmt.Sprintf(`%s\{method="%s"\} ([0-9\.]+)`, name, method)
		re := regexp.MustCompile(pattern)
		matches := re.FindSubmatch(body)

		if len(matches) < 2 {
			return 0.0
		}

		val, err := strconv.ParseFloat(string(matches[1]), 64)
		require.NoError(t, err)
		return val
	}

	// 4. Connect Client
	rpcClient, err := rpc.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)
	client := ethclient.NewClient(rpcClient)
	defer client.Close()

	method := "eth_getTransactionByHash"

	// Get initial values
	initialMisses := getMetric("ethereum_cache_misses_total", method)
	initialHits := getMetric("ethereum_cache_hits_total", method)

	// 5. Make a request (Cache Miss)
	_, _, err = client.TransactionByHash(context.Background(), common.HexToHash("0x123"))
	require.NoError(t, err)

	// Verify Miss incremented
	currentMisses := getMetric("ethereum_cache_misses_total", method)
	require.Equal(t, initialMisses+1, currentMisses, "Miss count should increment")

	// Verify Hit same
	currentHits := getMetric("ethereum_cache_hits_total", method)
	require.Equal(t, initialHits, currentHits, "Hit count should not change")

	// 6. Make same request (Cache Hit)
	_, _, err = client.TransactionByHash(context.Background(), common.HexToHash("0x123"))
	require.NoError(t, err)

	// Verify Miss same
	finalMisses := getMetric("ethereum_cache_misses_total", method)
	require.Equal(t, currentMisses, finalMisses, "Miss count should not change")

	// Verify Hit incremented
	finalHits := getMetric("ethereum_cache_hits_total", method)
	require.Equal(t, currentHits+1, finalHits, "Hit count should increment")
}

func TestPrometheusMetricsAuth(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// 3. Start Proxy Server with Auth
	proxyPort := "8091"
	authToken := "secret-metrics-token"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, authToken, 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Request without token -> 401
	resp, err := http.Get("http://localhost:" + proxyPort + "/metrics")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// 5. Request with wrong token -> 401
	req, _ := http.NewRequest("GET", "http://localhost:"+proxyPort+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// 6. Request with correct token -> 200
	req, _ = http.NewRequest("GET", "http://localhost:"+proxyPort+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}
