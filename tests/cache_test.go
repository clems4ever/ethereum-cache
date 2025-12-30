package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/server"
	"github.com/clems4ever/ethereum-cache/testdb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestCaching(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	var requestCount int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "eth_getTransactionByHash":
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"hash":"0x0000000000000000000000000000000000000000000000000000000000000123", "nonce":"0x0", "blockHash":"0x0000000000000000000000000000000000000000000000000000000000000000", "blockNumber":"0x1", "transactionIndex":"0x0", "from":"0x0000000000000000000000000000000000000000", "to":"0x0000000000000000000000000000000000000000", "value":"0x0", "gas":"0x0", "gasPrice":"0x0", "input":"0x", "v":"0x0", "r":"0x0", "s":"0x0"}}`, req.ID)))
		case "eth_getTransactionReceipt":
			// 512 zeros for logsBloom
			bloom := fmt.Sprintf("%0512d", 0)
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"transactionHash":"0x0000000000000000000000000000000000000000000000000000000000000123", "blockNumber":"0x1", "blockHash":"0x0000000000000000000000000000000000000000000000000000000000000000", "transactionIndex":"0x1", "type":"0x1", "status":"0x1", "cumulativeGasUsed":"0x1", "gasUsed":"0x1", "contractAddress":null, "logs":[], "logsBloom":"0x%s"}}`, req.ID, bloom)))
		case "eth_getStorageAt":
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"0x0000000000000000000000000000000000000000000000000000000000000001"}`, req.ID)))
		case "eth_getProof":
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"address":"0x0000000000000000000000000000000000000123","accountProof":[],"balance":"0x0","codeHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0","storageHash":"0x0000000000000000000000000000000000000000000000000000000000000000","storageProof":[]}}`, req.ID)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8085"
	srv := server.New(":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Connect using ethclient
	rpcClient, err := rpc.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)
	client := ethclient.NewClient(rpcClient)
	defer client.Close()

	// Test Cases
	txHash := common.HexToHash("0x123")
	addr := common.HexToAddress("0x123")
	blockNum := big.NewInt(100)

	// 5.1 eth_getTransactionByHash
	_, _, err = client.TransactionByHash(context.Background(), txHash)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	_, _, err = client.TransactionByHash(context.Background(), txHash)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount)) // Should still be 1

	// 5.2 eth_getTransactionReceipt
	_, err = client.TransactionReceipt(context.Background(), txHash)
	require.NoError(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))

	_, err = client.TransactionReceipt(context.Background(), txHash)
	require.NoError(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))

	// 5.3 eth_getStorageAt
	_, err = client.StorageAt(context.Background(), addr, common.Hash{}, blockNum)
	require.NoError(t, err)
	require.Equal(t, int32(3), atomic.LoadInt32(&requestCount))

	_, err = client.StorageAt(context.Background(), addr, common.Hash{}, blockNum)
	require.NoError(t, err)
	require.Equal(t, int32(3), atomic.LoadInt32(&requestCount))

	// 5.4 eth_getProof
	var result interface{}
	err = rpcClient.CallContext(context.Background(), &result, "eth_getProof", addr, []string{}, hexutil.EncodeBig(blockNum))
	require.NoError(t, err)
	require.Equal(t, int32(4), atomic.LoadInt32(&requestCount))

	err = rpcClient.CallContext(context.Background(), &result, "eth_getProof", addr, []string{}, hexutil.EncodeBig(blockNum))
	require.NoError(t, err)
	require.Equal(t, int32(4), atomic.LoadInt32(&requestCount))
}

func TestErrorHandling(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond with an error
		errorResponse := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"mock error"}}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(errorResponse))
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8086"
	srv := server.New(":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Connect ethclient to Proxy
	rpcClient, err := rpc.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)

	// 5. Call BlockNumber - Expecting an error
	var result string
	err = rpcClient.CallContext(context.Background(), &result, "eth_blockNumber")
	require.Error(t, err)
	require.Contains(t, err.Error(), "mock error")
}

func TestNoCachingForLatestBlock(t *testing.T) {
	// 1. Setup Test Database
	tdb := testdb.NewDatabase(t)
	db, err := database.NewDB(context.Background(), tdb.ConnString())
	require.NoError(t, err)
	defer db.Close()

	// 2. Setup Mock Upstream Ethereum Node
	var requestCount int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "eth_getStorageAt":
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"0x0000000000000000000000000000000000000000000000000000000000000001"}`, req.ID)))
		case "eth_getProof":
			w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"address":"0x0000000000000000000000000000000000000123","accountProof":[],"balance":"0x0","codeHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0","storageHash":"0x0000000000000000000000000000000000000000000000000000000000000000","storageProof":[]}}`, req.ID)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	// 3. Start Proxy Server
	proxyPort := "8087"
	srv := server.New(":"+proxyPort, upstream.URL, db, "", 0, 0, 0)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// 4. Connect using ethclient
	rpcClient, err := rpc.Dial("http://localhost:" + proxyPort)
	require.NoError(t, err)
	client := ethclient.NewClient(rpcClient)
	defer client.Close()

	addr := common.HexToAddress("0x123")

	// 5.1 eth_getStorageAt with latest block (nil)
	_, err = client.StorageAt(context.Background(), addr, common.Hash{}, nil)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	_, err = client.StorageAt(context.Background(), addr, common.Hash{}, nil)
	require.NoError(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount)) // Should be 2 (no cache)

	// 5.2 eth_getProof with latest block (nil)
	var result interface{}
	err = rpcClient.CallContext(context.Background(), &result, "eth_getProof", addr, []string{}, nil)
	require.NoError(t, err)
	require.Equal(t, int32(3), atomic.LoadInt32(&requestCount))

	err = rpcClient.CallContext(context.Background(), &result, "eth_getProof", addr, []string{}, nil)
	require.NoError(t, err)
	require.Equal(t, int32(4), atomic.LoadInt32(&requestCount)) // Should be 4 (no cache)
}
