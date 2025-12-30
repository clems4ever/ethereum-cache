package tests

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestCacheCleanup(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		// Echo back a result with specific length
		// We'll use eth_getStorageAt as it's cacheable
		// Request: {"method":"eth_getStorageAt", ...}
		// We return a result of length 100 bytes (hex string)
		result := fmt.Sprintf("0x%0200d", 0) // 202 chars = 202 bytes
		w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":"%s"}`, result)))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server with Cleanup
	// Each entry will be roughly:
	// Key: ~64 bytes (sha256 hex)
	// Method: "eth_getStorageAt" ~16 bytes
	// Response: ~250 bytes (json wrapper + result)
	// Result Length: 202 bytes
	// Overhead: 64 bytes per row (as per our calculation)
	// Total per row ~ 202 + 64 = 266 bytes (as per our calculation logic)

	// Let's set max size to allow 2 entries, but trigger cleanup on 3rd.
	// 2 * 266 = 532 bytes.
	// Set max size = 600 bytes.
	// Slack ratio = 0.5 (50%). So target size = 300 bytes.
	// When we add 3rd entry, total size ~ 800 bytes > 600.
	// Cleanup should reduce to <= 300 bytes.
	// This means removing 2 entries (since 2 entries ~ 532 > 300).
	// So only 1 entry should remain.

	proxyPort := "8088"
	maxSize := int64(600)
	slackRatio := 0.5
	srv := server.New(":"+proxyPort, upstream.URL, db, "", maxSize, slackRatio, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	client, err := ethclient.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)
	defer client.Close()

	// 4. Insert 3 entries
	// Test Cases
	// We need to use a specific block number to ensure caching happens
	blockNum := big.NewInt(100)

	// Entry 1
	_, err = client.StorageAt(context.Background(), common.HexToAddress("0x1"), common.Hash{}, blockNum)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Entry 2
	_, err = client.StorageAt(context.Background(), common.HexToAddress("0x2"), common.Hash{}, blockNum)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Entry 3 - This should trigger cleanup
	_, err = client.StorageAt(context.Background(), common.HexToAddress("0x3"), common.Hash{}, blockNum)
	require.NoError(t, err)
	// Wait for cleanup to happen (async)
	time.Sleep(500 * time.Millisecond)

	// 5. Verify Cache Size
	size, err := db.GetCacheSize(context.Background())
	require.NoError(t, err)

	// We expect size to be <= 300 (target size)
	// Each entry is 202 + 64 = 266 bytes.
	// So we expect 1 entry remaining (266 bytes).
	require.LessOrEqual(t, size, int64(300))
	require.Greater(t, size, int64(0))

	// 6. Verify which entries remain
	// Oldest accessed should be deleted first.
	// Entry 1 was accessed first, then 2, then 3.
	// So 1 and 2 should be deleted?
	// Wait, we sort by last_accessed_at ASC.
	// 1 is oldest, 2 is middle, 3 is newest.
	// We delete from oldest until we reach target.
	// To reach 300 from ~800, we need to free 500.
	// 1 entry = 266. 2 entries = 532.
	// So deleting 1 and 2 frees 532, leaving 266 (Entry 3).
	// So Entry 3 should be present. Entry 1 and 2 should be gone.

	// Check Entry 1 (should be gone - cache miss -> upstream call)
	// But wait, upstream call will re-populate cache.
	// We can check DB directly.

	var count int
	// We need to reconstruct keys or just count rows.
	err = tdb.Pool().QueryRow(context.Background(), "SELECT COUNT(*) FROM rpc_cache").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Verify the remaining one is Entry 3 (newest)
	// We can check by checking if we can retrieve it without upstream call?
	// Or just trust the count and size.
}
