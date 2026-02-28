# Admin gRPC API & astro-queen TUI

Operator tooling for managing a running Astro platform. The HTTP admin API has been replaced with a gRPC server on a separate port, accessible via the `astro-queen` terminal UI.

## Architecture

```
┌──────────────────────────────────────┐
│           astro-server               │
│                                      │
│  :8080  HTTP API  (user-facing)      │
│  :9091  gRPC admin  (operator-only)  │
└──────────────────────────────────────┘
            │ mTLS (optional)
            ▼
┌──────────────────────────────────────┐
│           astro-queen                │
│   k9s-style TUI — 4 views:           │
│   Deployments / Cluster / Images / Query │
└──────────────────────────────────────┘
```

The gRPC service is defined in `packages/astro-proto/proto/admin/v1/admin.proto`. Messages use JSON encoding over the gRPC transport (no protoc runtime required).

## astro-server configuration

| Env var | Default | Description |
|---|---|---|
| `ADMIN_GRPC_PORT` | `9091` | Port the admin gRPC server listens on |
| `ADMIN_GRPC_CERT_FILE` | — | Server TLS certificate (PEM). Leave unset to disable TLS |
| `ADMIN_GRPC_KEY_FILE` | — | Server TLS private key (PEM) |
| `ADMIN_GRPC_CA_FILE` | — | CA certificate used to verify client certs (mTLS) |

If all three cert vars are unset, the server starts without TLS and logs a warning. This is fine for local development, not suitable for production.

## Quick start (local, no TLS)

Add to `apps/astro-server/.env`:

```bash
ADMIN_GRPC_PORT=9091
```

Start the server and TUI in separate terminals:

```bash
# Terminal 1
moon run astro-server:dev

# Terminal 2
moon run astro-queen:run
```

`astro-queen` connects to `localhost:9091` by default.

## Production setup (mTLS)

### 1. Generate certificates

```bash
moon run astro-server:gen-admin-certs
```

This runs `apps/astro-server/scripts/gen-admin-certs.sh`, which:
- Generates a CA, server cert/key, and client cert/key into `apps/astro-server/.certs/`
- Copies client certs to `~/.astro-queen/`
- Writes `~/.astro-queen/config.yaml`

The `.certs/` directory is gitignored. Rotate certs annually (365-day validity by default).

### 2. Configure the server

The script prints the vars to add to `.env`:

```bash
ADMIN_GRPC_PORT=9091
ADMIN_GRPC_CERT_FILE=/path/to/.certs/server.crt
ADMIN_GRPC_KEY_FILE=/path/to/.certs/server.key
ADMIN_GRPC_CA_FILE=/path/to/.certs/ca.crt
```

### 3. Run

```bash
moon run astro-server:dev    # Terminal 1
moon run astro-queen:run-tls # Terminal 2
```

### Manual cert generation

If you need to generate certs without the script (e.g. using an existing CA):

```bash
# Server cert
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -subj "/CN=your-server-hostname"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt

# Client cert
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj "/CN=astro-queen"
openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt
```

## astro-queen config

Config file: `~/.astro-queen/config.yaml`

```yaml
server: "localhost:9091"       # gRPC server address
cert_file: "~/.astro-queen/client.crt"  # omit for insecure
key_file:  "~/.astro-queen/client.key"
ca_file:   "~/.astro-queen/ca.crt"
```

Override the server address at runtime:

```bash
ASTRO_QUEEN_SERVER=prod-host:9091 moon run astro-queen:run
# or
./bin/astro-queen --server prod-host:9091
```

## TUI reference

```
┌─ astro-queen  [1]Deployments [2]Cluster [3]Images [4]Query ──┐
│ status line                                                   │
│ ┌────────────────────────────────────────────────────────┐   │
│ │  Name   Namespace   Status   ...                       │   │
│ │  ...                                                   │   │
│ └────────────────────────────────────────────────────────┘   │
│ ↑↓ navigate  d delete  r restart  R refresh  Tab cycle  q quit│
└───────────────────────────────────────────────────────────────┘
```

### Global keys

| Key | Action |
|---|---|
| `1` / `2` / `3` / `4` | Switch to Deployments / Cluster / Images / Query view |
| `Tab` | Cycle to next view |
| `q` | Quit |

### Deployments view

Shows all active deployments across all accounts.

| Key | Action |
|---|---|
| `↑↓` | Navigate rows |
| `d` | Delete selected deployment (confirmation required) |
| `r` | Restart a pod in selected namespace (prompts for pod name) |
| `R` | Refresh |

### Cluster view

Shows live Kubernetes resource counts and details.

| Key | Action |
|---|---|
| `p` | Show Pods sub-tab |
| `k` | Show K8s Deployments sub-tab |
| `s` | Show Services sub-tab |
| `R` | Refresh |

### Images view

Shows ECR repositories matching the `{ENVIRONMENT}-tenant-*` prefix.

| Key | Action |
|---|---|
| `↑↓` | Navigate rows |
| `R` | Refresh |

### Query view

Execute raw SQL against the Astro database.

| Key | Action |
|---|---|
| `Enter` | Run query (when input is focused) |
| `/` or `i` | Focus the SQL input field |
| `↑↓` | Navigate result rows (when table is focused) |

## gRPC API reference

Defined in `packages/astro-proto/proto/admin/v1/admin.proto`.

| RPC | Request | Description |
|---|---|---|
| `ListDeployments` | `namespace` (optional filter) | All active deployments with account info |
| `GetClusterStatus` | `namespace` (optional filter) | K8s deployments, pods, services and summary |
| `ListImages` | — | ECR tenant repositories with tags |
| `DeleteDeployment` | `namespace` | Delete all K8s resources and mark DB record undeployed |
| `RestartDeployment` | `namespace`, `pod` | Delete pod so K8s recreates it |
| `QueryDatabase` | `query` | Execute raw SQL, returns columns and rows |

## Moon tasks

| Task | Description |
|---|---|
| `moon run astro-server:gen-admin-certs` | Generate dev mTLS certs, write `~/.astro-queen/config.yaml` |
| `moon run astro-queen:run` | Build and launch TUI (insecure, `localhost:9091`) |
| `moon run astro-queen:run-tls` | Build and launch TUI with `~/.astro-queen/config.yaml` |
| `moon run astro-queen:build` | Build binary only |
| `moon run astro-queen:link` | Symlink binary to `$GOBIN/astro-queen` |
| `moon run astro-proto:deps` | `go mod tidy` for the proto package |
| `moon run astro-proto:generate` | Re-run `buf generate` to regenerate Go code from proto |
