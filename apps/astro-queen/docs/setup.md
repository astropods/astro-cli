# Queen Setup Guide

Queen is the Astro admin toolkit. It ships as a single binary that embeds a web UI and proxies requests to the Astro admin gRPC server.

## Prerequisites

- Go 1.24+
- Bun (for building the frontend)
- Access to an Astro admin gRPC server

## Building

From the `apps/astro-queen` directory:

```sh
# Install frontend dependencies and build the SPA
cd web && bun install && bun run build && cd ..

# Build the Go binary (embeds the SPA)
go build -o bin/queen .
```

Or using moon from the repo root:

```sh
moon run astro-queen:start
```

This runs `web-install` -> `web-build` -> `build` -> `start` automatically.

## Configuration

Queen reads config from `~/.astro-queen/config.yaml`. Run the interactive setup wizard:

```sh
queen configure
```

This prompts for:

1. **Server address** - gRPC admin server (default: `admin.astropod.ai:9091`)
2. **Client certificate PEM** - for mTLS authentication
3. **Client key PEM** - for mTLS authentication
4. **CA certificate PEM** - for mTLS verification

The wizard writes cert files to `~/.astro-queen/` and generates `config.yaml`.

### Manual config

Create `~/.astro-queen/config.yaml`:

```yaml
server: "your-server:9091"
cert_file: ~/.astro-queen/client.crt
key_file: ~/.astro-queen/client.key
ca_file: ~/.astro-queen/ca.crt
```

All fields are optional:

| Field | Default | Description |
|---|---|---|
| `server` | `admin.astropod.ai:9091` | Admin gRPC server address |
| `cert_file` | (none) | Client cert path or inline PEM. Required for mTLS. |
| `key_file` | (none) | Client key path or inline PEM. Required for mTLS. |
| `ca_file` | (none) | CA cert path or inline PEM. Required for mTLS. |

If no cert files are provided, queen connects to gRPC insecurely (suitable for local dev).

## Starting the server

```sh
queen server
```

This starts an HTTP server on `http://127.0.0.1:8888` and opens your browser.

### Flags

| Flag | Default | Description |
|---|---|---|
| `-p, --port` | `8888` | HTTP server port |
| `-s, --server` | (from config) | Override gRPC server address |
| `-c, --config` | `~/.astro-queen/config.yaml` | Config file path |

Examples:

```sh
# Custom port
queen server --port 3000

# Point to a different gRPC server
queen server --server my-server:9091

# Use a different config file
queen server -c /path/to/config.yaml
```

## Web UI

The web UI has two sections:

### Admin
- **Deployments** - List all deployments, view details with cluster status, pods, logs, and environment variables. Delete or restart deployments.
- **Accounts** - List accounts with inline rename.
- **Agents** - List agents with expandable build history.
- **Cluster** - Query cluster status by namespace (pods, deployments, services, ingresses, events).

## Dev environment management

Queen also includes CLI commands for managing Astropod dev environments (no web UI for these):

```sh
queen devenv show              # Show your dev environment
queen devenv create <name>     # Create a new environment
queen devenv destroy <name>    # Destroy an environment
queen devenv reset-images <name>
queen devenv reset-db <name>
queen devenv restart <name>
queen devenv plan <name>       # Terraform dry run
queen devenv status <run-id>   # Check workflow status
```

## Frontend development

For iterating on the web UI without rebuilding the Go binary:

```sh
# Terminal 1: Start the Go server
queen server --port 8888

# Terminal 2: Start Vite dev server with proxy
cd apps/astro-queen/web
bun run dev
```

The Vite dev server (port 5173) proxies `/api` requests to the Go server on port 8888, giving you hot reload for frontend changes.

## Architecture

```
queen server [--port 8888]
  -> Go HTTP server on 127.0.0.1:8888
  -> /api/admin/*      -> proxies to gRPC AdminService (mTLS)
  -> /*                -> serves embedded React SPA (go:embed)
  -> opens browser automatically
```

The binary is self-contained - the React SPA is embedded at build time via `go:embed`, so there are no external file dependencies at runtime.
