# astro-server

Go backend for the Astro platform using the Gin framework. Handles agent registry, deployment to Kubernetes, and authentication.

## Local Development

**Prerequisites:** Docker Desktop with Kubernetes enabled (Settings > Kubernetes > Enable Kubernetes), Bun, and a `psql` client on your PATH (used by the migration step). Atlas and air are auto-installed on first run.

```bash
moon run astro-server:dev
```

This single command:
1. Copies `.env.example` to `.env` if `.env` doesn't exist (you must then set `DATABASE_URL` and real WorkOS credentials - see Configuration)
2. Installs [air](https://github.com/air-verse/air) (hot reload) and [Atlas](https://atlasgo.io) (migrations) if missing
3. Applies the schema and River migrations against `DATABASE_URL`
4. Starts the server with hot reload on `http://localhost:8080`

The dev database is **remote** - nothing local starts Postgres; point `DATABASE_URL` at your dev database. Ctrl+C stops the server.

### Configuration

`.env.example` is a starting point, not turnkey: you must set `DATABASE_URL` and drop in real stage WorkOS credentials (the committed values are placeholders). Authentication is always on, and local login is routed to the stage/preview WorkOS app - a separate identity from your production `astropods.com` login. Key settings:

| Variable          | Description                                      | Local value                                                         |
| ----------------- | ------------------------------------------------ | ------------------------------------------------------------------- |
| `DATABASE_URL`    | Connection string for your (remote) dev database | `.env.example` ships a `localhost:5432` placeholder - replace it    |
| `K8S_CLIENT_MODE` | `"local"` for kubeconfig, `"eks"` for IRSA       | `local`                                                             |
| `KUBE_CONTEXT`    | Kubernetes context to use                        | `docker-desktop`                                                    |
| `REGISTRY_URL`    | Container registry URL                           | `docker.io/library`                                                 |
| `WORKOS_API_KEY`, `WORKOS_CLIENT_ID` | WorkOS app credentials (stage app)   | placeholders - use real stage values                               |
| `AUTH_COOKIE_PASSWORD` | Key for session-cookie encryption           | set in `.env.example`                                              |
| `FGA_SHADOW_ENABLED` | Compare deployment authorization with WorkOS without enforcing it | `false` |
| `PORT`            | Server port                                      | `8080`                                                             |
| `GIN_MODE`        | Gin framework mode                               | `release`                                                          |
| `LOG_LEVEL`       | Logging level (`debug`, `info`, `warn`, `error`) | `debug`                                                            |

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

### Local CLI

Build the dev CLI (`ast-dev`, pointed at your local server) with Moon - see the [repo README](../../README.md) and the [local-development runbook](../../docs/04-guides/local-development.md):

```bash
moon run astro-cli:link      # builds ast-dev, symlinks it into ~/go/bin
```

(The server's `/install` and `/download/:name` routes, gated on `DOWNLOAD_BASE_URL`, are the production CLI distribution path, not part of local dev.)

## Moon Tasks

```bash
moon run astro-server:dev              # Local dev (migrations + hot reload)
moon run astro-server:build            # Build binary to bin/astro-server
moon run astro-server:test             # Run unit tests
moon run astro-server:test-integration # Integration tests (needs Postgres)
moon run astro-server:e2e              # K8s e2e (vcluster + Postgres)
moon run astro-server:typecheck        # Type/compile check
moon run astro-server:fmt              # Format code
moon run astro-server:vet              # Run go vet
moon run astro-server:lint             # Run golangci-lint
moon run astro-server:deps             # Download and tidy dependencies
```

### CI test jobs

Path filters in `.github/workflows/test.yml` only fan out when application paths change — workflow-only edits do not run these jobs. The astro-server jobs and their local equivalents:

| CI job | Build tags | Local command |
| ------ | ---------- | ------------- |
| `Test Go applications (astro-server)` | (default) | `moon run astro-server:test` |
| `Integration tests (astro-server + Postgres)` | `integration` | `moon run astro-server:test-integration` |
| `K8s integration tests (vcluster + Postgres)` | `k8s` | `moon run astro-server:e2e` (full stack) |

Unit and integration suites use `gotestsum`; lint uses `golangci-lint-action` with the shared config at repo root (10m timeout on CI for cold-cache runs). Integration and K8s e2e jobs enable `-race` on main pushes only (PRs skip it for speed).

## Endpoints

### Health Probes

| Endpoint       | Purpose                                                   |
| -------------- | --------------------------------------------------------- |
| `GET /livez`   | Liveness — is the process alive?                          |
| `GET /readyz`  | Readiness — can it serve traffic?                         |
| `GET /healthz` | Verbose health check (database, K8s connectivity, uptime) |

### Authentication

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

**Protected (auth required):**

| Endpoint                                      | Description             |
| --------------------------------------------- | ----------------------- |
| `POST /api/v1/agents/register`                | Register agent          |
| `POST /api/v1/deploy`                         | Deploy agent to K8s     |
| `POST /api/v1/undeploy`                       | Undeploy agent from K8s |
| `GET /api/v1/deployments`                     | List deployments        |
| `GET /api/v1/deployments/:name/:version/logs` | Get pod logs            |

**Admin (basic auth required):**

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
├── scripts/
│   └── dev.sh                 # Local dev orchestration (migrations + air)
├── .air.toml                  # Hot reload config
├── .env.example               # Local dev defaults
├── moon.yml                   # Moon task runner config
├── go.mod
└── go.sum
```

SQL migrations live at repo-root `sql/astro-server/` (Atlas declarative schema) and `sql/river/` (queue migrations), not under `astro-server/`.

## Production Deployment

### Docker

```dockerfile
FROM golang:1.25-alpine AS builder
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
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
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
| `WORKOS_API_KEY`       | WorkOS API key                          |
| `WORKOS_CLIENT_ID`     | WorkOS client ID                        |
| `AUTH_COOKIE_PASSWORD` | Min 32 chars for AES-256-GCM encryption |
| `AUTH_COOKIE_SECURE`   | `true` (HTTPS-only cookies)             |
| `GIN_MODE`             | `release`                               |
