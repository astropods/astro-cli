# Per-deployment network metrics API

## Summary

Beyla started emitting eBPF-derived HTTP/DB/RPC RED metrics for every agent pod into the central Prometheus, but the frontend had no way to read them. This change adds a three-endpoint API on astro-server, plus TanStack Query hooks on astro-client, so the agent detail page can render inbound traffic, outbound calls, database operations, and charts over time. The metrics surface as **aggregated flow rollups**, not per-request distributed traces — Beyla does not emit trace IDs today (Tempo wiring is deferred).

## Design

- **Three directions, one model.** All endpoints accept `direction=inbound|outbound|database`. v1 covers HTTP server / HTTP client / DB client only; RPC is deferred (see Known limitations). The per-direction `directionSpec` (`apps/astro-server/handlers/network.go`) is a list so additional metric families can be unioned in once they're proven compatible.

- **Filter strategy.** Every PromQL query filters on `agent="{sanitized-account}.{sanitized-agent}"` (the existing `astro.dev/agent` pod label set by `deployment.AgentLabelValue`) plus the configured `cluster` label. No pod-regex fallback — the label is already attached by `deployment.GenerateLabels` on every agent workload.

- **Endpoint shapes:**
  - `GET /deployments/:id/network/summary` — RED aggregates per direction over a window (default last 1 h). Six parallel instant queries per direction via `errgroup`: request count, error count, P50/P95/P99 latency, unique peer count, total bytes.
  - `GET /deployments/:id/network/flows?direction=…` — top-N rows ranked by `requests | latency_p95 | errors`. A single `sum by (peer, status_code)` query per direction yields request counts, error counts, and the 2xx/4xx/5xx breakdown in one round trip; latency and bytes follow in parallel.
  - `GET /deployments/:id/network/timeseries?direction=…&metric=…` — bucketed series for charts. Accepts `group_by=peer|status_class`. Peer breakdowns are server-capped at 8 series via `topk(8, …)` so high-cardinality outbound destinations cannot blow up the response. Status-class folding (raw codes → `2xx`/`4xx`/`5xx`) happens in Go after the matrix arrives.

- **`promquery.QueryRange`.** The existing client wrapped only `/api/v1/query`. Added a parallel `QueryRange` plus a `MatrixSample`/`Point` shape so the timeseries endpoint can hit `/api/v1/query_range` without pulling in the Prom Go SDK.

- **Auth.** A new `resolveDeploymentContext` helper does the same JWT → deployment → account-membership dance as `resolveLangfuseContext`. The two are parallel by design until a third caller earns a shared abstraction.

- **Frontend.** `useNetworkSummary`, `useNetworkFlows`, `useNetworkTimeseries` mirror the observability hooks; cache settings split between live (`staleTime: 0`) for summary/timeseries and table (`staleTime: 5m`) for flows. Window range flows in as RFC3339 query params; group-by and sort knobs are typed unions matching the server validation.

## Known limitations

- **RPC not yet surfaced.** RPC metric families (`rpc_{server,client}_duration_seconds`) are intentionally excluded from v1. OTel semconv gives RPC a different label set than HTTP (`rpc.service`/`rpc.method` vs `http.route`/`server.address`) and the histogram bucket boundaries aren't guaranteed to match across families, so unioning them into the HTTP directions produced misleading `by (peer)` rollups and unsafe `histogram_quantile` results. RPC will land as its own `direction=rpc` once labels and buckets are validated against live Beyla output.
- **Bun outbound HTTPS.** Beyla does not yet instrument outbound HTTPS from Bun runtimes — those agents will show inbound and database flows but no outbound traffic. UI consumers should render the empty state, not assume "no traffic."
- **Database errors.** Beyla's `db_client_operation_duration_seconds` has no status label, so `errors`/`error_rate` on the database direction stays zero. The schema fields exist for forward compat.

## Migration

No action required. Endpoints are additive; existing observability and knowledge-store metric routes are untouched.
