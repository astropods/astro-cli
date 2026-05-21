# Deployment

Dockerfiles and moon tasks for building Astro service images. 
Used by CI and to support `ast dev --local`.

With `ast dev --local`, the CLI runs the agent as a local process, using local Docker images and packages from **`ASTRO_SOURCE`**. Those packages must have **`dist/`** built first.

All instructions assume you are in the repo root.

## Build packages

```bash
bun install
bun run build
moon run messaging:sdk-build
moon run adapters:build
```

## Build images

| File | Image | Source |
|------|--------|--------|
| `Dockerfile.astro-collector` | prod-astro-collector | OTel Collector custom distribution |
| `Dockerfile.astro-registry` | astro-registry | apps/astro-registry |
| `Dockerfile.astro-server` | astro-server | apps/astro-server |
| `Dockerfile.astro-client` | astro-client | apps/astro-client (SSR); built by CI |

Build context for collector, registry, server, and client is the workspace root (so `COPY packages/...` works).
Note that `adapters`, `messaging`, and `playground` are git submodules; run `git submodule update --init --recursive` after clone.

```bash
# Messaging sidecar → astro-messaging:latest (bundles the playground as well)
moon run deployment:messaging

# Collector (OpenTelemetry collector) → prod-astro-collector:latest
moon run deployment:collector
```

## Rmove local images

```bash
# Remove local/ghcr.io astro images (server, registry, prod-astro-collector)
moon run deployment:clean
```
