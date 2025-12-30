# Ethereum Cache Proxy

A high-performance caching proxy for Ethereum JSON-RPC requests, written in Go. It caches responses in a PostgreSQL database to reduce calls to your upstream provider (e.g., Infura, Alchemy) and improve latency for repeated queries.

## Features

- **Caching**: Caches JSON-RPC responses in PostgreSQL.
- **Rate Limiting**: Limits the request rate to the upstream provider to avoid overages.
- **Authentication**: Protects the proxy and metrics endpoints with Bearer token authentication.
- **Metrics**: Exposes Prometheus metrics for cache hits, misses, size, and item count.
- **Health Check**: Public endpoint for health monitoring.
- **Automatic Cleanup**: Background process to evict old cache entries when the size limit is reached.
- **Structured Logging**: Uses Zap for high-performance, structured logging.

## Configuration

The application is configured via a YAML file or environment variables. See `config.example.yaml` for a template.

| Key | Env Var | Description | Default |
|-----|---------|-------------|---------|
| `port` | `PORT` | The port to listen on. | `8080` |
| `upstream_url` | `UPSTREAM_URL` | The URL of the upstream Ethereum RPC provider. | Required |
| `database_dsn` | `DATABASE_DSN` | PostgreSQL connection string. | Required |
| `auth_token` | `AUTH_TOKEN` | Secret token for Bearer authentication. | Empty (No auth) |
| `max_cache_size_bytes` | `MAX_CACHE_SIZE_BYTES` | Maximum size of the cache in bytes. | `0` (Unlimited) |
| `cleanup_slack_ratio` | `CLEANUP_SLACK_RATIO` | Fraction of cache to clear when limit is reached (0.0-1.0). | `0.2` |
| `rate_limit` | `RATE_LIMIT` | Max requests per second to upstream. | `0` (Unlimited) |

## Getting Started

### Prerequisites

- Go 1.25+
- PostgreSQL 16+
- Docker & Docker Compose (optional)

### Running with Docker Compose

1. Create a configuration file:
   ```bash
   cp config.example.yaml .config.yaml
   ```
   Edit `.config.yaml` with your upstream URL and other settings.

2. Start the services:
   ```bash
   docker-compose up --build
   ```

The application will be available at `http://localhost:8085` (as configured in `docker-compose.yml`).

### Running Locally

1. Start a PostgreSQL database.

2. Build the application:
   ```bash
   go build -o bin/ethereum-cache cmd/app/main.go
   ```

3. Run the application:
   ```bash
   ./bin/ethereum-cache --config config.example.yaml
   ```

## API Endpoints

### `POST /`
The main JSON-RPC proxy endpoint. Forwards requests to the upstream provider if not cached.

**Headers:**
- `Content-Type: application/json`
- `Authorization: Bearer <auth_token>` (if configured)

**Example:**
```bash
curl -X POST http://localhost:8080/ \
  -H "Authorization: Bearer your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

### `GET /metrics`
Exposes Prometheus metrics.

**Headers:**
- `Authorization: Bearer <auth_token>` (if configured)

**Metrics:**
- `ethereum_cache_hits_total`: Total number of cache hits.
- `ethereum_cache_misses_total`: Total number of cache misses.
- `ethereum_cache_size_bytes`: Current size of the cache in bytes.
- `ethereum_cache_items_count`: Current number of items in the cache.

### `GET /health`
Public health check endpoint. Returns `200 OK` if the service is running.

## Development

### Running Tests
```bash
go test -v ./...
```

### Linting
```bash
golangci-lint run
```
