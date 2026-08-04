# Deployment

Dockerfiles and moon tasks for building Astro service images.
Used by CI, and locally to build the sidecar/service images that `ast dev` and local-mode K8s deployments consume.

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
| `Dockerfile.astro-collector` | `collector` (tagged `astropods/collector`) | OTel Collector custom distribution |
| `Dockerfile.astro-registry` | `astro-registry` | apps/astro-registry |
| `Dockerfile.astro-server` | `astro-server` | apps/astro-server |
| `Dockerfile.astro-client` | `astro-client` | apps/astro-client (SSR); built by CI |
| `Dockerfile.astro-otel` | `astro-otel` | apps/astro-otel (OTLP ingest); built by CI |

Build context for collector, registry, server, and client is the workspace root (so `COPY packages/...` works).
Note that `adapters` and `messaging` are git submodules; run `git submodule update --init --recursive` after clone.

```bash
# Messaging sidecar → messaging:latest (also tagged astropods/messaging:latest)
moon run deployment:messaging

# Collector (OpenTelemetry Collector) → collector:latest (also tagged astropods/collector:latest)
moon run deployment:collector
```

## Remove local images

```bash
# Remove local astro images (client, server, registry, collector, messaging)
moon run deployment:clean
```
