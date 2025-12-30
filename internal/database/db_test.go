package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB(t *testing.T) {
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	t.Run("Set and Get Cached Result", func(t *testing.T) {
		key := "test-key-1"
		method := "eth_test"
		response := []byte(`{"result":"success"}`)

		// Set
		err := db.SetCachedRPCResult(ctx, key, method, response)
		require.NoError(t, err)

		// Get
		cached, err := db.GetCachedRPCResult(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, response, cached)
	})

	t.Run("Get Non-Existent Result", func(t *testing.T) {
		cached, err := db.GetCachedRPCResult(ctx, "non-existent-key")
		require.NoError(t, err)
		assert.Nil(t, cached)
	})

	t.Run("Update Existing Result", func(t *testing.T) {
		key := "test-key-2"
		method := "eth_test"
		response1 := []byte(`{"result":"1"}`)
		response2 := []byte(`{"result":"2"}`)

		// Set initial
		err := db.SetCachedRPCResult(ctx, key, method, response1)
		require.NoError(t, err)

		// Update
		err = db.SetCachedRPCResult(ctx, key, method, response2)
		require.NoError(t, err)

		// Get
		cached, err := db.GetCachedRPCResult(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, response2, cached)
	})

	t.Run("Check Result Length", func(t *testing.T) {
		key := "test-key-length"
		method := "eth_test"
		response := []byte("12345") // length 5

		err := db.SetCachedRPCResult(ctx, key, method, response)
		require.NoError(t, err)

		// Verify length in DB directly
		var length int
		err = tdb.Pool().QueryRow(ctx, "SELECT result_length FROM rpc_cache WHERE key = $1", key).Scan(&length)
		require.NoError(t, err)
		assert.Equal(t, 5, length)
	})

	t.Run("Last Accessed At Update", func(t *testing.T) {
		key := "test-key-access"
		method := "eth_test"
		response := []byte(`{}`)

		err := db.SetCachedRPCResult(ctx, key, method, response)
		require.NoError(t, err)

		// Get initial last_accessed_at
		var initialAccess time.Time
		err = tdb.Pool().QueryRow(ctx, "SELECT last_accessed_at FROM rpc_cache WHERE key = $1", key).Scan(&initialAccess)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond) // Ensure time difference

		// Get via API, which should update last_accessed_at
		_, err = db.GetCachedRPCResult(ctx, key)
		require.NoError(t, err)

		// Get new last_accessed_at
		var newAccess time.Time
		err = tdb.Pool().QueryRow(ctx, "SELECT last_accessed_at FROM rpc_cache WHERE key = $1", key).Scan(&newAccess)
		require.NoError(t, err)

		assert.True(t, newAccess.After(initialAccess), "last_accessed_at should be updated")
	})
}
