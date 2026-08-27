# Queen Setup Guide

Queen is the Astro admin toolkit. It ships as a single binary that embeds a web UI and proxies requests to astro-server's admin gRPC service.

## Prerequisites

- Go 1.24+
- Bun (for building the frontend)
- For prod/preview: access to the **Astro** 1Password vault (item `queen-bee-client`) — see [`../../../docs/04-guides/queen-mtls-certificate-generation.md`](../../../docs/04-guides/queen-mtls-certificate-generation.md)

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
moon run astro-queen:build   # web-install -> web-build -> build
moon run astro-queen:link    # same, then symlinks bin/queen onto $PATH as `queen`
```

## Authenticating (prod / preview)

```sh
queen login
```

Prompts for your 1Password account name, fetches the shared `queen-bee-client` mTLS cert/key/CA from the **Astro** vault, and writes them to `~/.astro-queen/{client.crt,client.key,ca.crt}` plus `~/.astro-queen/config.yaml` (which stores only the 1Password account name — there's no server address or cert path to configure manually; those are conventional paths and per-environment constants, not user settings).

## Starting the admin UI

```sh
queen local   admin   # localhost:9091, insecure (no certs needed)
queen preview admin   # admin.astropod.ai:443, mTLS
queen prod    admin   # admin.astropods.ai:443, mTLS
```

Each starts an HTTP server on `http://127.0.0.1:8888` (override with `-p/--port`) and opens your browser. `--no-open` skips that.

`local` connects insecurely and needs no `queen login` step — it talks to a locally-running astro-server that has `ADMIN_GRPC_PORT` set and no TLS configured. `preview`/`prod` require `queen login` to have run first.

## Web UI

The web UI covers far more than deployments and cluster status:

- **Deployments** — list/inspect across all accounts with live pod/container detail, view logs and env vars, delete, restart a pod, stop/wake up, rollback, redeploy (also the mechanism for cross-cluster migration — see the architecture doc), repair a stored spec.
- **Accounts** — list/rename/delete/purge, billing detail and provisioning repair (Metronome, Langfuse, Bifrost), per-account cluster placement bindings, cache invalidation.
- **Clusters** — registered-cluster health, deregistration, pull-secret refresh.
- **Cluster status** — live per-namespace K8s state (pods, deployments, services, ingresses, events).
- **Jobs** — browse/cancel/retry River jobs, pause/resume queues, trigger a job kind ad hoc.
- **Alerts**, **Audit findings**, **Quota requests**, **Blueprints** (agents), **Feedback**, **Migrations** (read-only cluster-migration audit trail), **API Client** (generic OpenAPI explorer against astro-server's customer-facing API, authenticated as yourself via device login).

See [`../../../docs/03-architecture/astro-queen.md`](../../../docs/03-architecture/astro-queen.md) for how these fit together, the RPC surface, and the auth model.

## Dev environment management

Queen also includes CLI commands for managing Astropod dev environments via `astro-infra` GitHub Actions workflows (no web UI for these):

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
moon run astro-queen:dev
```

Runs `air` (Go hot-reload on the server) and the Vite dev server (port 5173, proxying `/api` to the Go server on 8888) concurrently. Equivalently by hand: `./dev.sh`, or `queen local admin --port 8888` in one terminal and `cd web && bun run dev` in another.

## Architecture

```
queen <local|preview|prod> admin [--port 8888] [--no-open]
  -> Go HTTP server on 127.0.0.1:8888 (stdlib net/http, no framework)
  -> /api/admin/*  -> translates to one gRPC call against astro-server's
                      AdminService (mTLS for preview/prod, insecure+loopback
                      for local)
  -> /api/astro/*  -> reverse proxy to astro-server's normal customer-facing
                      API, authenticated as you via device login (API Client page)
  -> /*            -> serves the embedded React SPA (go:embed), falling back
                      to index.html for client-side routing
  -> opens browser automatically
```

The binary is self-contained — the React SPA is embedded at build time via `go:embed`, so there are no external file dependencies at runtime. The gRPC server it talks to is not a separate service: it's a second `grpc.Server` running inside the same astro-server process as the public API, on its own port.
