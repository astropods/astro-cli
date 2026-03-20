## Summary

Adds the active agent monitoring dashboard with real-time metrics, trace viewer, and deployment history tab.

## Design

- **ActiveDetailView.tsx**: Two-tab layout — Monitor (KPI cards + Recharts time-series chart + trace list) and Deployments (past builds with status). Fetches live metrics via existing query hooks. Chart uses `ComposedChart` with an `Area` for request volume and a `Line` for error rate.
- **Deployments tab → container accordions**: Each pod container (agent, messaging, collector, etc.) loads logs via `useDeploymentLogs` when the accordion is open, using the same `/api/v1/deployments/:id/logs` contract as `PodLogViewer`. Time-range options match the API (`15m`–`7d`); a per-container refresh control triggers `refetch()`.
- **Deployments tab → history table**: Rows come from `GET .../deployment/history` (`useDeploymentHistory`), merged with the live deployment if missing from the response. Search, status filter, and date preset (all / 7d / 30d) apply client-side. **Duration**: live row = time since `deployed_at` (with fallback to list `created_at` for that id); past rows = `undeployed_at − deployed_at` when set, otherwise span until the **next newer** row’s `deployed_at` (sorted newest-first). Shown as seconds when &lt;1m, then minutes/hours/days. Kebab **Redeploy** opens Configure; **View pod logs** expands the live row and first container; **Rollback** is disabled until a backend exists.
- **Monitor tab**: Request chart’s second series is labeled **Avg latency** and plots `avg_latency_ms` from metrics buckets (true P95 remains in KPI cards from the summary endpoint). Token **input/output** counts are summed from bucket `input_tokens` / `output_tokens` when present; otherwise the UI shows summary **total** only with a short note (no fabricated 60/40 split).
- **Header status badge**: Reflects `mapDeploymentStatus` (Live / Deploying / Error / Inactive) instead of always “Active”.
- **useDeploymentLogs**: Optional `enabled` flag so UIs can lazy-fetch when a panel opens.
- **package.json**: Adds `recharts ^3.8.0` for the metrics chart.

## Migration

Run `bun install` after pulling to pick up the recharts dependency.

**Monitor (traces/metrics)** in local dev requires working Galileo env on `astro-server` (`GALILEO_API_KEY`, `GALILEO_PROJECT_ID`, etc.). If those are missing or invalid, the UI shows an observability banner; **Deployments → pod logs** still use Kubernetes/Loki from the server.

**Vite:** `astro-trading-card` + `uqr` are listed under `ssr.noExternal` / `optimizeDeps.include` so SSR dev does not fail resolving `uqr`.
