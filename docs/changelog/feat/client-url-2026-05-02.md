## Summary

Adds a local development script that starts the entire platform (Traefik, server, client) in one command and builds a local `ast-dev` CLI pointed at `http://localhost`. Also fixes `ast push` to show the correct blueprint URL in the web UI rather than the API server URL.

## Design

**Local dev script (`scripts/local-dev.sh`)** starts three services and builds the CLI:
- Traefik on port 80 as a reverse proxy: `/api` routes to the server at `:8080`, everything else to the client at `:5173`
- `astro-server` and `astro-client` via their existing moon dev tasks
- `ast-dev` CLI via `moon run astro-cli:build` with `ASTRO_SERVER_URL=http://localhost`

**Traefik config (`traefik/dynamic/services.yml`)** adds a second router for `PathPrefix(/api)` at higher priority than the catch-all client router, so both services share `localhost:80` without any path rewriting needed (the server already mounts routes under `/api`).

**`DefaultServerURL` in moon build** is now injectable via `ASTRO_SERVER_URL` env var, defaulting to `http://localhost:8080` (direct). The local-dev script passes `http://localhost` (via Traefik). This lets `moon run astro-cli:build` work correctly in both contexts without a separate moon task.

## Migration

No action required. The existing `moon run astro-client:dev` and `moon run astro-server:dev` workflows continue to work unchanged.
