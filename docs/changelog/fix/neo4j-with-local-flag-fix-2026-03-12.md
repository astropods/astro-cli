# Fix `ast dev --local` connectivity to sidecar containers

## Summary

When running `ast dev --local`, the agent process executes directly on the host via `bun run` rather than inside a Docker container. Two bugs prevented the agent from connecting to knowledge-store sidecars like Neo4j:

1. Environment variables like `NEO4J_HOST` were set to Docker-internal service names (e.g. `knowledge-graph`) that only resolve inside the Docker network — not from the host.
2. Provider extra ports (e.g. Neo4j Bolt on 7687, Qdrant gRPC on 6334) were never published to the host, so even with `localhost` the connection would fail on non-default protocols.

## Design

**Host rewrite (`buildLocalAgentEnv` in `dev.go`):** A new `rewriteDockerHostsToLocalhost` function runs after `BuildEnvironment` populates the agent env map. It collects all Docker service names from the spec (`model-*`, `knowledge-*`, `tool-*`) by checking which entries deploy containers, then rewrites any env value that matches or contains a service name to use `localhost` instead. This handles both bare `_HOST` values and embedded hostnames in `_URL`/`_BASE_URL` values.

**Extra port publishing (`BuildProject` in `builder.go`):** After the primary port mapping block for knowledge services, provider `ExtraPorts` are now appended to the service's published ports. This is generic — any provider that declares extra ports gets them published automatically.

## Migration

No user action required. Existing `astropods.yml` specs work as before. Agents using `ast dev --local` with Neo4j, Qdrant, or other providers with extra ports will now connect correctly without manual `.env` overrides.
