# astro-server

Go backend for the Astro platform using the Gin framework. Handles agent registry, deployment to Kubernetes, and authentication.

## Local Development

**Prerequisites:** Docker Desktop with Kubernetes enabled (Settings > Kubernetes > Enable Kubernetes).

```bash
moon run astro-server:dev
```

This single command:
1. Copies `.env.example` to `.env` (if `.env` doesn't exist)
2. Installs [air](https://github.com/air-verse/air) for hot reload (if not installed)
3. Starts Postgres via Docker Compose
4. Runs database migrations
5. Starts the server with hot reload on `http://localhost:4321`

Ctrl+C stops everything (server + Postgres).

### Configuration

The `.env.example` is preconfigured for local dev — it works out of the box. Key settings:

| Variable          | Description                                      | Local Default                                                       |
| ----------------- | ------------------------------------------------ | ------------------------------------------------------------------- |
| `DATABASE_URL`    | Postgres connection string                       | `postgres://postgres:postgres@localhost:5432/astro?sslmode=disable` |
| `K8S_CLIENT_MODE` | `"local"` for kubeconfig, `"eks"` for IRSA       | `local`                                                             |
| `KUBE_CONTEXT`    | Kubernetes context to use                        | `docker-desktop`                                                    |
| `KUBECONFIG`      | Path to kubeconfig file                          | `~/.kube/config`                                                    |
| `REGISTRY_URL`    | Container registry URL                           | `docker.io/library`                                                 |
| `AUTH_ENABLED`    | Enable WorkOS authentication                     | `false`                                                             |
| `PORT`            | Server port                                      | `4321`                                                              |
| `GIN_MODE`        | Gin framework mode                               | `debug`                                                             |
| `LOG_LEVEL`       | Logging level (`debug`, `info`, `warn`, `error`) | `debug`                                                             |

For EKS production mode, uncomment and set the EKS variables in `.env`:

```bash
K8S_CLIENT_MODE=eks
EKS_CLUSTER_NAME=my-eks-cluster
K8S_MASTER_URL=https://xxx.eks.amazonaws.com
AWS_REGION=us-east-1
REGISTRY_URL=123456789.dkr.ecr.us-east-1.amazonaws.com
```

### Image Handling

- **Local mode (`K8S_CLIENT_MODE=local`):** Images use `IfNotPresent` — locally-built images (agent, messaging, collector) are used as-is, while third-party images (qdrant, redis, neo4j) are pulled from Docker Hub on first use.
- **EKS mode (`K8S_CLIENT_MODE=eks`):** Images are always pulled from the configured registry (`PullAlways`). If `PROXY_REGISTRY_HOST` is set, image references are rewritten from the proxy registry to ECR.

### Testing the /dev Page and CLI Download

1. Build CLI binaries into a local directory:
   ```bash
   mkdir -p apps/astro-server/cli
   cd apps/astro-cli
   GOOS=darwin GOARCH=arm64 go build -o ../astro-server/cli/ast-darwin-arm64 .
   ```
2. Add `CLI_DIR=./cli` to your `.env`
3. Start the frontend in another terminal:
   ```bash
   cd apps/astro-client && bun run dev
   ```
4. Open `http://localhost:5173/dev`

## Moon Tasks

```bash
moon run astro-server:dev      # Start local dev (Postgres + migrations + hot reload)
moon run astro-server:build    # Build binary to bin/astro-server
moon run astro-server:test     # Run tests
moon run astro-server:fmt      # Format code
moon run astro-server:vet      # Run go vet
moon run astro-server:lint     # Run golangci-lint
moon run astro-server:deps     # Download and tidy dependencies
```

### CI test jobs

Path filters in `.github/workflows/test.yml` only fan out when application paths change — workflow-only edits do not run these jobs. The astro-server jobs and their local equivalents:

| CI job | Build tags | Local command |
| ------ | ---------- | ------------- |
| `Test Go applications (astro-server)` | (default) | `moon run astro-server:test` |
| `Integration tests (astro-server + Postgres)` | `integration` | `moon run astro-server:test-integration` |
| `K8s integration tests (vcluster + Postgres)` | `k8s` | `moon run astro-server:e2e` (full stack) |

Unit and integration suites use `gotestsum`; lint uses `golangci-lint-action` with the shared config at repo root (10m timeout on CI for cold-cache runs).

## Endpoints

### Health Probes

| Endpoint       | Purpose                                                   |
| -------------- | --------------------------------------------------------- |
| `GET /livez`   | Liveness — is the process alive?                          |
| `GET /readyz`  | Readiness — can it serve traffic?                         |
| `GET /healthz` | Verbose health check (database, K8s connectivity, uptime) |

### Authentication (when `AUTH_ENABLED=true`)

| Endpoint             | Description                              |
| -------------------- | ---------------------------------------- |
| `GET /auth/login`    | Redirect to WorkOS AuthKit               |
| `GET /auth/callback` | OAuth callback, sets session cookie      |
| `GET /auth/logout`   | Clear session, redirect to WorkOS logout |
| `GET /auth/me`       | Get current user info                    |
| `POST /auth/refresh` | Refresh session token                    |

### API v1

**Public:**

| Endpoint                            | Description       |
| ----------------------------------- | ----------------- |
| `GET /api/v1/health`                | Health check      |
| `GET /api/v1/ready`                 | Readiness check   |
| `GET /api/v1/agents`                | List agents       |
| `GET /api/v1/agents/:name`          | Get agent         |
| `GET /api/v1/agents/:name/:version` | Get agent version |

**Protected (auth required when enabled):**

| Endpoint                                      | Description             |
| --------------------------------------------- | ----------------------- |
| `POST /api/v1/agents/register`                | Register agent          |
| `POST /api/v1/deploy`                         | Deploy agent to K8s     |
| `POST /api/v1/undeploy`                       | Undeploy agent from K8s |
| `GET /api/v1/deployments`                     | List deployments        |
| `GET /api/v1/deployments/:name/:version/logs` | Get pod logs            |

**Admin (basic auth required when enabled):**

| Endpoint                           | Description               |
| ---------------------------------- | ------------------------- |
| `GET /api/v1/admin/cluster/status` | Cluster resource overview |
| `GET /api/v1/admin/images`         | List ECR images           |

### Authentication Methods

1. **Session cookie** (web clients): Set automatically after `/auth/login`
2. **Bearer token** (API clients): `Authorization: Bearer <access_token>`

## Project Structure

```
astro-server/
├── main.go                    # Entry point, router setup
├── handlers/                  # HTTP request handlers
│   ├── agents.go              # Agent registry
│   ├── auth.go                # Authentication (WorkOS)
│   ├── cluster.go             # Admin cluster status
│   ├── deploy.go              # Deploy/undeploy/list/logs
│   ├── health.go              # Health check
│   ├── probes.go              # K8s liveness/readiness probes
│   └── readiness.go           # Readiness check
├── internal/
│   ├── agentindex/            # Agent index (Postgres)
│   ├── auth/                  # JWT, sessions, WorkOS SDK
│   ├── config/                # Env-based configuration
│   ├── deployment/            # Spec translation, validation, naming
│   ├── k8s/                   # Kubernetes client layer
│   │   ├── client.go          # ClusterClient interface + factory
│   │   ├── eks.go             # EKS/IRSA implementation
│   │   ├── local.go           # Kubeconfig implementation (Docker Desktop/kind/minikube)
│   │   ├── applier.go         # Create/update K8s resources
│   │   ├── deleter.go         # Delete K8s resources
│   │   ├── deployment.go      # Deployment manifest builder
│   │   ├── statefulset.go     # StatefulSet manifest builder
│   │   ├── image_resolver.go  # Proxy registry → ECR rewriting
│   │   └── ...
│   ├── logger/                # Structured logging (slog)
│   └── middleware/            # Auth, CORS, logging, recovery, security headers
├── migrations/                # SQL migrations
├── scripts/
│   └── dev.sh                 # Local dev orchestration
├── docker-compose.yml         # Postgres + migrations for local dev
├── .air.toml                  # Hot reload config
├── .env.example               # Local dev defaults
├── moon.yml                   # Moon task runner config
├── go.mod
└── go.sum
```

## Production Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN go build -o astro-server .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/astro-server .
CMD ["./astro-server"]
```

### Kubernetes Health Probes

```yaml
livenessProbe:
  httpGet:
    path: /livez
    port: 4321
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 4321
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Production Environment Variables

In addition to the variables above, production requires:

| Variable               | Description                             |
| ---------------------- | --------------------------------------- |
| `K8S_CLIENT_MODE`      | `eks`                                   |
| `EKS_CLUSTER_NAME`     | EKS cluster name                        |
| `K8S_MASTER_URL`       | K8s API server endpoint                 |
| `REGISTRY_URL`         | ECR registry URL                        |
| `AUTH_ENABLED`         | `true`                                  |
| `WORKOS_API_KEY`       | WorkOS API key                          |
| `WORKOS_CLIENT_ID`     | WorkOS client ID                        |
| `AUTH_COOKIE_PASSWORD` | Min 32 chars for AES-256-GCM encryption |
| `AUTH_COOKIE_SECURE`   | `true` (HTTPS-only cookies)             |
| `GIN_MODE`             | `release`                               |
