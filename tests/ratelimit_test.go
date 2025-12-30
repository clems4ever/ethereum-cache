package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/stretchr/testify/require"
)

func TestRateLimiting(t *testing.T) {
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

	// 3. Start Proxy Server with Rate Limit
	// Rate Limit = 1 request per second. Burst = 2.
	proxyPort := "8089"
	rateLimit := 1.0
	srv := server.New(":"+proxyPort, upstream.URL, db, "", 0, 0, rateLimit)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	client := &http.Client{}
	url := "http://localhost:" + proxyPort

	// Helper to send request
	sendRequest := func() int {
		req, _ := http.NewRequest("POST", url, bytes.NewBufferString(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// 4. Send requests
	// Burst is 2. So first 2 should succeed.
	code1 := sendRequest()
	require.Equal(t, http.StatusOK, code1, "Request 1 should succeed")

	code2 := sendRequest()
	require.Equal(t, http.StatusOK, code2, "Request 2 should succeed")

	// 3rd request should fail immediately if sent fast enough
	code3 := sendRequest()
	require.Equal(t, http.StatusTooManyRequests, code3, "Request 3 should be rate limited")

	// Wait for 1 second to replenish tokens
	time.Sleep(1100 * time.Millisecond)

	// Should succeed again
	code4 := sendRequest()
	require.Equal(t, http.StatusOK, code4, "Request 4 should succeed after waiting")
}
