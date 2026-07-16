# Fix: token totals on the agent detail panel

## Summary

The chat detail panel showed "0" for token usage on every agent. The panel's
"Tokens" figure falls back to the per-deployment observability summary
(`GET /api/v1/deployments/:id/observability/summary`) when the cached account
summary has no token series for that deployment. That summary endpoint always
returned `total_tokens: 0`, so the fallback was dead and the panel read zero.

## Design

Langfuse's trace list carries no per-trace token usage, so the summary handler,
which is built from the trace list, hardcoded `total_tokens: 0`. The reliable
source for token totals is the Langfuse daily-metrics endpoint: it aggregates
input and output usage per day, and the observability summary refresh worker
already uses it to build the account-level token series.

`GetLangfuseSummary` now fetches daily metrics alongside the trace list and sums
`InputTokens() + OutputTokens()` across the window, matching the refresh worker.
The value flows into `computeLangfuseSummary`, which takes a `totalTokens`
argument instead of the hardcoded zero. A daily-metrics failure is non-fatal:
latency and trace counts still render, tokens fall back to zero.

The frontend was already correct: it renders whatever the API returns, so no
client change was needed once the endpoint reports real totals.

## Migration

None. The summary response shape is unchanged; `total_tokens` now carries a real
value instead of always zero.
