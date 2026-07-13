# feat: per-model cost and latency breakdown on Insights

## Summary

There was no per-model view of what an agent's account actually spends and how each model performs, so understanding where cost and latency go per model was impossible. This surfaces a per-model breakdown on the Insights page. Closes #1374.

## Design

The account observability summary already carried per-model cost (`cost_by_model`). This extends that breakdown and renders it:

- **Backend.** The per-model entries in the account summary now also carry token volume (`total_tokens` + `token_pct`), request count, and p50/p95 latency. Cost and tokens are rolled up from the existing daily metrics; request count and latency come from one added Langfuse observations query grouped by model with no time dimension (so the percentiles are computed over the whole period rather than averaged across days, which would be wrong). The query is fail-open: if it fails, those fields are zero and the cost/token breakdown still renders. (p50 is returned but the UI shows only p95; see below.)
- **Frontend.** A "Models" view on the Insights page lists each model's request count, spend, spend share (a "% Total" column), token volume, and p95 latency. It lives as a third toggle in the existing View-by control (People / Agents / Models) and is defined alongside the agents and people views in `TopSpendersTable`, which now share a single table-shell helper so the three views render one common chrome. Model ids show the clean family name with the full id (including any date/version suffix) preserved in a hover tooltip. The models view self-fetches the account summary for the selected window (its data comes from a different endpoint than the agents/people rows).

Per review, a single tail-latency column (p95) is shown rather than both p50 and p95, and the suggested-cheaper-model callout was held back until the recommendation heuristic is more reliable.

## Migration

None. This is additive; existing observability responses gain fields and the Insights View-by control gains a Models option.
