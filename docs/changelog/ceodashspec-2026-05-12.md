## Summary

Adds a spec for the CEO Dashboard (Activity) feature: an account-scoped view of agent usage, cost, and model consumption over time. This PR contains only the spec — no production code changes.

## Design

The dashboard is built around two new server endpoints:

- **`GET /observability/summary?from=&to=`** — patches the existing handler to accept a time range and return pre-aggregated data: stat card values, % change vs the prior equal-length period, daily averages, cost-over-time (stacked by model), and cost-by-model share. When `from`/`to` are absent ("All"), the `change` key is omitted and the client hides % change badges.
- **`GET /observability/blueprints-summary?from=&to=`** — new endpoint that fans out per-deployment Langfuse calls (capped at 10 concurrent via `errgroup.SetLimit`) and returns per-blueprint cost, tokens, requests, and P95 latency sorted by spend descending.

All aggregation and percentage math happens on the server. The client receives shaped, render-ready objects from both hooks (`useAccountActivitySummary`, `useBlueprintsSummary`).

The patched summary endpoint is a breaking shape change: `total_traces` / `input_tokens` / `output_tokens` at root are replaced by `totals.requests` / `totals.input_tokens` etc. PR 1 must migrate `DashboardStats.tsx` and `AccountObservabilitySummaryResponse` in the same change.

## Migration

No migration required. This is a spec-only PR.
