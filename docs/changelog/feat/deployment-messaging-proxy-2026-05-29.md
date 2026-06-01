## Summary

Browsers and other Astro-authenticated clients can chat with messaging-enabled agents through astropod.ai without opening the agent subdomain or completing a second WorkOS login on the messaging ALB. astro-server validates the existing Astro session, proxies REST and SSE traffic to the deployment's messaging sidecar, and stamps the WorkOS user ID upstream so the messaging container's auth model stays unchanged.

## Design

- **Route** — `ANY /api/v1/deployments/:id/messaging/*` on the protected API group. Path segments after `messaging/` map to the messaging web adapter under `/api/` (e.g. `conversations`, `conversations/{id}/stream`, `api/agent/config`).
- **Session gate** — Same auth chain as other deployment routes: Astro session required, deployment lookup, account membership check via `resolveDeployment`.
- **Upstream identity** — Proxy injects `x-amzn-oidc-identity` with the authenticated user's WorkOS ID. Messaging continues to run its existing header session manager and `/deployments/authorize` grant checks; the Astro session cookie is not forwarded.
- **Upstream resolution** — Local dev uses existing `MESSAGING_URL_OVERRIDE`. Production resolves the deployment's cluster via `deploymentClusterClient`, reads the `{agent}-messaging` Service HTTP port, and reaches the sidecar through the Kubernetes API service proxy (`/api/v1/namespaces/{ns}/services/{svc}:{port}/proxy/...`). This works for EU and other remote clusters without requiring cross-cluster ClusterIP routing from the primary astro-server pods.
- **Streaming** — SSE responses (`text/event-stream`) are flushed line-by-line with write deadlines cleared, matching the deployment log stream pattern. Non-streaming responses are copied through with hop-by-hop headers stripped.
- **Out of scope** — Subdomain Launch URLs are unchanged. WebSocket audio is not proxied yet. astro-client does not call these routes; any consumer must implement the messaging REST/SSE protocol against the proxy paths.

## Migration

No user or operator action required. Deploy astro-server to enable the routes. For local testing, set `MESSAGING_URL_OVERRIDE` to a reachable messaging web adapter (e.g. `http://localhost:8090`).
