# Insights perf pass: batched Langfuse queries + hardening

## Summary

Insights page paints faster on org-switch and date-range change because the two
account-level summary endpoints stopped fanning out per deployment for the bulk
of their work. Adds `total_tokens` as the canonical token field across the
summary responses, hardens the account-summary handler with a request timeout
and fan-out cap, and reorders the users-view table columns.

## Design

**Account-summary endpoint** (`GetAccountLangfuseSummary`). The per-deployment
`/metrics/daily` fan-out (`mergedDailyMetrics`, N calls + N prior-period calls
= 2N) is replaced with two batched `/api/public/metrics` queries
(`accountDailyMetrics`):

- Traces view, grouped by `[tags, time]` — drives per-day trace count and the
  active-deployment set. Active deps are inferred from the tag dimension
  (any deployment with `count > 0`).
- Observations view, grouped by `[providedModelName, time]` — drives per-day,
  per-model cost + input/output token breakdown that powers the CostByModel
  donut and CostOverTime stacked bars.

Net: 2N → 3 calls per request (Q_traces + Q_obs + optional Q_prior on bounded
windows). Prior-period query stays fail-open: if it times out the change %
tiles render `—` but the page still loads.

**Blueprints-summary endpoint** (`GetAccountBlueprintsSummary`). The
per-deployment P95 trace query (N calls) becomes one batched `/metrics` call
grouped by tags (`batchedP95Latencies`). The per-deployment `/metrics/daily`
fan-out stays because the observations view does NOT support `tags` as a
grouping dimension — only as a filter — and we need per-(deployment, model)
cost to compute `top_model` per agent. Net: 2N → N+1.

**Request timeout + fan-out cap** on `GetAccountLangfuseSummary`: 30s context
timeout and `maxDeployments = 100` on the visible-tag list, matching the
blueprints-summary handler's defenses. Prevents a slow Langfuse upstream from
pinning workers and bounds the worst-case fan-out for the deployments we still
hit per-request.

**`total_tokens` as the source of truth.** Adds `total_tokens` to
`AccountSummaryTotals`, `BlueprintSummaryEntry`, and `BlueprintDailyTokens`.
Clients should prefer it. `input_tokens` / `output_tokens` stay on the response
shape (still populated by the per-deployment daily path) but are 0 in views
that source tokens from the traces view (combined-only). Drops the old
`input_tokens: totalTokens, output_tokens: 0` hack in `recomputeTotalsFromUsers`.

**Frontend.** `StatCards` and `DashboardStats` prefer `total_tokens` with an
input+output fallback. `use-insights-data.ts` reads `b.total_tokens` rather
than summing `input + output`.

## Migration

None. `total_tokens` is additive; existing consumers continue to work via
input+output fallback.
