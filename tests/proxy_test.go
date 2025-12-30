package tests

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProxy(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple mock response for eth_blockNumber
		// Request: {"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}
		// Response: {"jsonrpc":"2.0","id":1,"result":"0x1234"}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1234"}`))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8087"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// 4. Connect ethclient to Proxy
	rpcClient, err := rpc.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)

	// We need a concrete implementation of ethclient that uses this rpcClient
	// ethclient.NewClient wraps rpc.Client
	// Let's try to use a basic rpc call first to verify connectivity.

	var result string
	err = rpcClient.CallContext(context.Background(), &result, "eth_blockNumber")
	require.NoError(t, err)
	require.Equal(t, "0x1234", result)
}

func TestAuthentication(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server with Auth
	proxyPort := "8088"
	authToken := "secret-token"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, authToken, 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Test Unauthorized Access
	client := &http.Client{}
	req, _ := http.NewRequest("POST", "http://localhost:"+proxyPort, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 5. Test Authorized Access
	req, _ = http.NewRequest("POST", "http://localhost:"+proxyPort, nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	// We need a valid body for the proxy to process, otherwise it might fail before auth check?
	// Actually auth check is first in the middleware.
	// But let's send a valid json rpc body to be sure we get a 200 OK from upstream
	req.Body = io.NopCloser(bytes.NewBufferString(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`))

	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestEthClient(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1234"}`))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8089"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Connect using ethclient
	client, err := ethclient.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)
	defer client.Close()

	// 5. Call BlockNumber
	bn, err := client.BlockNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(0x1234), bn)
}

type headerTransport struct {
	T       http.RoundTripper
	Headers map[string]string
}

func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Headers {
		req.Header.Set(k, v)
	}
	return h.T.RoundTrip(req)
}

func TestEthClientWithAuth(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1234"}`))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server with Auth
	proxyPort := "8090"
	authToken := "secret-token"
	srv := server.New(zap.NewNop(), ":"+proxyPort, upstream.URL, db, authToken, 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Connect using ethclient with Auth Header
	httpClient := &http.Client{
		Transport: &headerTransport{
			T: http.DefaultTransport,
			Headers: map[string]string{
				"Authorization": "Bearer " + authToken,
			},
		},
	}
	rpcClient, err := rpc.DialHTTPWithClient("http://localhost:"+proxyPort, httpClient)
	require.NoError(t, err)
	client := ethclient.NewClient(rpcClient)
	defer client.Close()

	// 5. Call BlockNumber
	bn, err := client.BlockNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(0x1234), bn)
}
