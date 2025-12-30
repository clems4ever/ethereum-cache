package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/clems4ever/ethereum-cache/internal/cleanup"
	"github.com/clems4ever/ethereum-cache/internal/database"
	"github.com/clems4ever/ethereum-cache/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Handler struct {
	logger         *zap.Logger
	upstreamURL    string
	db             *database.DB
	httpClient     *http.Client
	cleanupManager *cleanup.Manager
	limiter        *rate.Limiter
}

func NewHandler(logger *zap.Logger, upstreamURL string, db *database.DB, cleanupManager *cleanup.Manager, rateLimit float64) *Handler {
	var limiter *rate.Limiter
	if rateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit)+1)
	}
	return &Handler{
		logger:         logger,
		upstreamURL:    upstreamURL,
		db:             db,
		httpClient:     &http.Client{},
		cleanupManager: cleanupManager,
		limiter:        limiter,
	}
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Check if cacheable
	if isCacheable(req.Method, req.Params) {
		key, err := generateCacheKey(req.Method, req.Params)
		if err == nil {
			cached, err := h.db.GetCachedRPCResult(r.Context(), key)
			if err == nil && cached != nil {
				// Cache hit
				metrics.CacheHits.WithLabelValues(req.Method).Inc()
				resp := JSONRPCResponse{
					JSONRPC: "2.0",
					Result:  cached,
					ID:      req.ID,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			metrics.CacheMisses.WithLabelValues(req.Method).Inc()
		}
	}

	// Forward to upstream
	if h.limiter != nil {
		if err := h.limiter.Wait(r.Context()); err != nil {
			http.Error(w, "upstream rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", h.upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")

	upstreamResp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	respBody, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusInternalServerError)
		return
	}

	// If cacheable, store result
	if isCacheable(req.Method, req.Params) {
		var resp JSONRPCResponse
		if err := json.Unmarshal(respBody, &resp); err == nil && resp.Error == nil {
			key, err := generateCacheKey(req.Method, req.Params)
			if err == nil {
				// We ignore error here as we want to return the response anyway
				if err := h.db.SetCachedRPCResult(r.Context(), key, req.Method, resp.Result); err == nil {
					if h.cleanupManager != nil {
						h.cleanupManager.NotifyWrite()
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(respBody)
}

func isCacheable(method string, params json.RawMessage) bool {
	switch method {
	case "debug_traceTransaction", "eth_getTransactionByHash", "eth_getTransactionReceipt":
		return true
	case "eth_getStorageAt":
		// params: [address, position, blockNumber]
		return isBlockNumberSpecific(params, 2)
	case "eth_getProof":
		// params: [address, storageKeys, blockNumber]
		return isBlockNumberSpecific(params, 2)
	default:
		return false
	}
}

func isBlockNumberSpecific(params json.RawMessage, index int) bool {
	var args []interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return false
	}
	if len(args) <= index {
		return false // Default is latest
	}
	blockParam, ok := args[index].(string)
	if !ok {
		return false // Should be string
	}
	return blockParam != "latest" && blockParam != "pending" && blockParam != "earliest"
}

func generateCacheKey(method string, params json.RawMessage) (string, error) {
	var args []interface{}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &args); err != nil {
			return "", err
		}
	}

	normalized := normalizeForCache(args)
	argsBytes, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(append([]byte(method), argsBytes...))
	return hex.EncodeToString(hash[:]), nil
}

func normalizeForCache(v any) any {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		type pair struct {
			K string `json:"k"`
			V any    `json:"v"`
		}
		pairs := make([]pair, len(keys))
		for i, k := range keys {
			pairs[i] = pair{K: k, V: normalizeForCache(t[k])}
		}
		return pairs
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = normalizeForCache(val)
		}
		return out
	default:
		return v
	}
}
