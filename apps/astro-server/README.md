# astro-server

A production-ready Go backend HTTP server for the Astro platform using the Gin framework.

## Features

- **HTTP Server**: Built with Gin framework for high performance
- **Structured Logging**: JSON-formatted structured logging using Go's `log/slog`
- **Graceful Shutdown**: Handles SIGINT and SIGTERM signals with configurable timeout
- **Middleware Stack**:
  - Recovery middleware for panic handling
  - Request logging with duration, status, and metadata
  - CORS support with configurable origins
  - Security headers (X-Frame-Options, CSP, HSTS, etc.)
- **Configuration**: Environment variable-based configuration with validation
- **Health Checks**:
  - `/api/v1/health` - Health check endpoint
  - `/api/v1/ready` - Readiness check for orchestration systems
- **Production Best Practices**:
  - Configurable timeouts (read, write, idle)
  - Trusted proxy configuration
  - Version-aware API routing
  - HTTP server with proper timeout settings

## Prerequisites

- Go 1.21 or higher
- Make (optional, but recommended)

## Quick Start

### Using Make (Recommended)

```bash
cd apps/astro-server
make run
```

### Using Go directly

```bash
cd apps/astro-server
go run main.go
```

The server will start on `http://localhost:8080` by default.

## Configuration

The application is configured using environment variables. Copy the example configuration file:

```bash
cp .env.example .env
```

### Environment Variables

| Variable | Description | Default | Options |
|----------|-------------|---------|---------|
| `PORT` | Server port | `8080` | Any valid port number |
| `HOST` | Server host | `0.0.0.0` | Any valid host/IP |
| `GIN_MODE` | Gin framework mode | `release` | `debug`, `release`, `test` |
| `READ_TIMEOUT` | HTTP read timeout | `10s` | Duration string (e.g., `30s`, `1m`) |
| `WRITE_TIMEOUT` | HTTP write timeout | `10s` | Duration string |
| `SHUTDOWN_TIMEOUT` | Graceful shutdown timeout | `30s` | Duration string |
| `LOG_LEVEL` | Logging level | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | Log output format | `json` | `json`, `text` |
| `ALLOWED_ORIGINS` | CORS allowed origins | `*` | Comma-separated list or `*` |
| `TRUSTED_PROXIES` | Trusted proxy IPs | (empty) | Comma-separated IP list |

### Example Configuration

```bash
# Development
PORT=8080
GIN_MODE=debug
LOG_LEVEL=debug
LOG_FORMAT=text
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173

# Production
PORT=8080
GIN_MODE=release
LOG_LEVEL=info
LOG_FORMAT=json
ALLOWED_ORIGINS=https://yourdomain.com
TRUSTED_PROXIES=10.0.0.0/8
READ_TIMEOUT=30s
WRITE_TIMEOUT=30s
```

## Endpoints

### Health Check

```
GET /api/v1/health
```

Returns the health status of the server.

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Readiness Check

```
GET /api/v1/ready
```

Returns the readiness status for orchestration systems (Kubernetes, Docker Swarm, etc.).

**Response (Ready):**
```json
{
  "status": "ready",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (Not Ready):**
```json
{
  "status": "not ready",
  "timestamp": "2024-01-15T10:30:00Z"
}
```
Status Code: 503

### Deploy Agent

```
POST /api/v1/deploy
```

Deploys an AI agent to Kubernetes from the agent index.

**Request Body:**
```json
{
  "name": "customer-support-agent",
  "version": "1.0.0",
  "user_credentials": {
    "OPENAI_API_KEY": "sk-...",
    "DATABASE_URL": "postgres://..."
  },
  "k8s_namespace": "production"
}
```

**Parameters:**
- `name` (required): Agent name
- `version` (required): Agent version to deploy
- `user_credentials` (optional): Map of credential keys to values required by the agent
- `k8s_namespace` (required): Target Kubernetes namespace for deployment

**Response (Success):**
```json
{
  "status": "success",
  "name": "customer-support-agent",
  "version": "1.0.0",
  "k8s_namespace": "production",
  "deployed_at": "2024-01-15T10:30:00Z",
  "resources": [
    {
      "kind": "Deployment",
      "name": "customer-support-agent-agent",
      "namespace": "production",
      "status": "created"
    },
    {
      "kind": "Service",
      "name": "customer-support-agent-agent",
      "namespace": "production",
      "status": "created"
    }
  ],
  "service_endpoints": [
    {
      "name": "http",
      "type": "ClusterIP",
      "url": "customer-support-agent-agent.production.svc.cluster.local",
      "port": 8080
    }
  ]
}
```

**Response (Validation Error):**
```json
{
  "error": "spec validation failed",
  "validation_errors": [
    {
      "field": "runtime.image",
      "message": "image is required"
    }
  ],
  "missing_credentials": ["OPENAI_API_KEY"]
}
```
Status Code: 400

**Response (Agent Not Found):**
```json
{
  "error": "agent version not found",
  "details": "customer-support-agent:1.0.0 not found in index"
}
```
Status Code: 404

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "name": "customer-support-agent",
    "version": "1.0.0",
    "k8s_namespace": "production",
    "user_credentials": {
      "OPENAI_API_KEY": "sk-..."
    }
  }'
```

### Undeploy Agent

```
POST /api/v1/undeploy
```

Removes a deployed agent from Kubernetes.

**Request Body:**
```json
{
  "name": "customer-support-agent",
  "version": "1.0.0",
  "k8s_namespace": "production"
}
```

**Parameters:**
- `name` (required): Agent name
- `version` (required): Agent version to undeploy
- `k8s_namespace` (required): Target Kubernetes namespace

**Response (Success):**
```json
{
  "status": "success",
  "name": "customer-support-agent",
  "version": "1.0.0",
  "k8s_namespace": "production",
  "undeployed_at": "2024-01-15T11:00:00Z",
  "resources": [
    {
      "kind": "Deployment",
      "name": "customer-support-agent-agent",
      "namespace": "production",
      "status": "deleted"
    },
    {
      "kind": "Service",
      "name": "customer-support-agent-agent",
      "namespace": "production",
      "status": "deleted"
    }
  ]
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/undeploy \
  -H "Content-Type: application/json" \
  -d '{
    "name": "customer-support-agent",
    "version": "1.0.0",
    "k8s_namespace": "production"
  }'
```

## Make Commands

```bash
make help         # Show all available commands
make deps         # Download dependencies
make build        # Build the application
make run          # Run the application
make test         # Run tests
make test-cover   # Run tests with coverage
make clean        # Remove build artifacts
make fmt          # Format code
make lint         # Run linter (requires golangci-lint)
make vet          # Run go vet
make all          # Run all build steps (clean, deps, fmt, vet, build)
```

## Development

### Install Dependencies

```bash
make deps
# or
go mod download
```

### Build

```bash
make build
# or
go build -o astro-server
```

### Run

```bash
make run
# or
go run main.go
# or build and run
make build && ./astro-server
```

### Run Tests

```bash
make test
# or with coverage
make test-cover
```

## Project Structure

```
astro-server/
├── main.go                    # Application entry point
├── handlers/                  # HTTP request handlers
│   ├── health.go             # Health check handler
│   ├── readiness.go          # Readiness check handler
│   └── deploy.go             # Agent deployment handlers
├── internal/
│   ├── config/               # Configuration management
│   │   └── config.go
│   ├── logger/               # Structured logging
│   │   └── logger.go
│   └── middleware/           # HTTP middleware
│       ├── logging.go        # Request logging
│       ├── recovery.go       # Panic recovery
│       └── security.go       # CORS and security headers
├── .env.example              # Example environment variables
├── .gitignore
├── Makefile                  # Build automation
├── README.md
├── go.mod
└── go.sum
```

## Production Deployment

### Docker

Create a `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN go build -o astro-server main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/astro-server .
EXPOSE 8080
CMD ["./astro-server"]
```

Build and run:

```bash
docker build -t astro-server .
docker run -p 8080:8080 \
  -e GIN_MODE=release \
  -e LOG_LEVEL=info \
  astro-server
```

### Kubernetes

The `/api/v1/ready` endpoint can be used for Kubernetes readiness probes:

```yaml
readinessProbe:
  httpGet:
    path: /api/v1/ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

livenessProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30
```

### Systemd Service

Create `/etc/systemd/system/astro-server.service`:

```ini
[Unit]
Description=Astro Server
After=network.target

[Service]
Type=simple
User=astro
WorkingDirectory=/opt/astro-server
EnvironmentFile=/opt/astro-server/.env
ExecStart=/opt/astro-server/astro-server
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable astro-server
sudo systemctl start astro-server
sudo systemctl status astro-server
```

## Graceful Shutdown

The server handles `SIGINT` and `SIGTERM` signals for graceful shutdown:

1. Stops accepting new connections
2. Waits for existing requests to complete (up to `SHUTDOWN_TIMEOUT`)
3. Closes the server
4. Logs shutdown completion

Test graceful shutdown:

```bash
# Start server
make run

# In another terminal, send SIGTERM
pkill -SIGTERM astro-server
```

## Monitoring

### Structured Logs

All logs are structured in JSON format (default) for easy parsing by log aggregation tools:

```json
{
  "time": "2024-01-15T10:30:00.123Z",
  "level": "INFO",
  "msg": "Request completed",
  "method": "GET",
  "path": "/api/v1/health",
  "status": 200,
  "duration": 2,
  "ip": "127.0.0.1"
}
```

### Adding Metrics

To add Prometheus metrics, integrate the `prometheus/client_golang` package:

```bash
go get github.com/prometheus/client_golang/prometheus/promhttp
```

Then add a metrics endpoint in `main.go`.

## Security

The server implements several security best practices:

- **Security Headers**: X-Frame-Options, CSP, HSTS, X-Content-Type-Options
- **CORS**: Configurable allowed origins
- **Trusted Proxies**: Support for load balancers and reverse proxies
- **Panic Recovery**: Graceful handling of panics with stack traces
- **Timeouts**: Read, write, and idle timeouts to prevent slowloris attacks
- **Graceful Shutdown**: Prevents request loss during deployments

## Troubleshooting

### Server won't start

Check if the port is already in use:

```bash
lsof -i :8080
```

### CORS errors

Ensure `ALLOWED_ORIGINS` is set correctly:

```bash
export ALLOWED_ORIGINS=http://localhost:3000
make run
```

### Verbose logging

Set log level to debug:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=text
make run
```

## Contributing

1. Format code: `make fmt`
2. Run linter: `make lint`
3. Run tests: `make test`
4. Build: `make build`

## License

[Your License Here]
