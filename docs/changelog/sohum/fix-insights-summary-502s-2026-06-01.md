# Insights: cache all three endpoints + degrade gracefully on Langfuse failure


## Summary

Preview was alerting on `>10%` 5xx for `/api/v1/accounts/:account/observability/summary` (and `>5%` server-wide). Root cause is upstream — Langfuse on `preview-primary-eks` fails its ClickHouse DNS lookup. astro-server was correctly propagating that as `502`, but the alert is firing on a downstream-only incident and the user sees a hard error state on Insights.

This PR doesn't fix the Langfuse outage. It does two things to make us invisible to it:

1. **Graceful degradation** — when Langfuse fails, the three Insights handlers return `200 OK` with `metrics_unavailable: true` and the page renders a banner instead of erroring.
2. **6h Redis cache + periodic refresh worker** — every Insights endpoint reads from Redis first; a new `InsightsRefreshWorker` pre-warms the cache every 6h. The worker only writes on Langfuse success, so the cached value survives multi-hour outages.

## Design

### Graceful degradation

Three response types pick up a `metrics_unavailable` boolean. When the compute path returns `ErrAllLangfuseCallsFailed`, the handler swaps `502` for `200 OK` with a zero-valued payload and the flag set. The Insights page reads the flag and surfaces a `WarningPanel` above the KPI cards; the rest of the page stays usable.

Two different semantics apply, by handler shape:

- **`/summary` and `/users-summary`** — sub-queries that fail are tallied; the flag fires only when *every* Langfuse call failed. Partial successes render normally with missing fields (existing fail-open per-field behavior preserved).
- **`/deployments-summary`** — same all-failed semantics, but with a wider surface: P95 batched query + N per-deployment daily-metrics calls + Q_tags inversion + (when `include_archived=true`) a second errgroup for tombstones. The tally covers every Langfuse call across both errgroups.

This avoids "any-failed" semantics that would surface the banner for a single transient row.

### Cache + worker

New package `internal/insightscache` mirrors the existing `internal/obssummary` pattern: thin `Get`/`Put` over the shared Redis client; `NoopCache` fallback when Redis isn't configured. Cache stores raw JSON bytes so the response shape can evolve without invalidating older entries. Key shape:

```
astro:insights:<endpoint>:<account_id>:group_by=<gb>:include_archived=<bool>
```

New River worker `InsightsRefreshWorker` (`internal/riverqueue/insights_refresh.go`) runs every `RefreshInterval` (1h). It enumerates accounts via `deploymentStore.ListAllActive` (matching `ObsSummaryRefreshWorker`), filters to accounts with Langfuse provisioned, calls the three shared `Compute*` functions, and writes the JSON results to Redis. **Writes are skipped on `ErrAllLangfuseCallsFailed`** — that's what lets cached entries survive a multi-hour upstream outage.

The list of `(endpoint, params)` tuples the worker refreshes lives in a single `insightsRefreshTargets` slice so the worker's writer keys and the handler's reader keys can't drift.

Handlers grew a cache-read prefix for canonical (no bounded period) requests:

```go
if !hasPeriod {
    if bytes, ok := insightscache.Get(ctx, cache, acct.ID, endpoint, params); ok {
        c.Data(http.StatusOK, "application/json", bytes)
        return
    }
}
```

Bounded-period requests skip the cache and run live.

### Breaking the import cycle

`handlers` already imports `riverqueue` (GitHub-build worker). The worker calling back into `handlers` would close the cycle, so an `InsightsSummaryComputer` interface lives on `riverqueue.Config`. `handlers.NewInsightsSummaryComputer()` returns a concrete implementation; `main` wires it during startup. The worker depends only on the interface.

### Fan-out: discovery job + per-account jobs

The refresh worker pair is split: `InsightsRefreshWorker` is now the
discovery job — it enumerates accounts that have Langfuse provisioned
(via `langfuseStore.ListAccountIDs`, which catches accounts that *used*
to have deployments but currently don't, unlike a deployments-table
join) and enqueues one `InsightsRefreshAccountArgs` per account. River
runs those in parallel up to the queue's MaxWorkers, retries individual
failures independently, and isolates one slow account from the rest.

Per-account error handling:
- `insightscache.ErrAllLangfuseCallsFailed` → return nil from the worker (no River retry; sustained Langfuse outages are re-attempted by the next discovery tick, not by retry storms)
- Transient errors (DB, etc.) → return the error so River retries with backoff
- Successful endpoints write their cache entry even when a sibling fails

### Admin cache invalidation

`InvalidateAccountCaches` (the existing queen-driven gRPC) now also wipes
the Insights cache entries for that account, so an operator can force a
fresh fetch without waiting on the 6h cycle. Mirrors the existing
deployment-cache invalidation pattern; no proto change.

### Tests

Added in this PR:

- `observability_insights_cache_test.go` — cache-hit returns cached bytes verbatim without touching Langfuse (verified via a sentinel HTTP server that fails the test on any request); `LangfuseFails` returns `200 OK` with `metrics_unavailable: true` for all three handlers; partial-failure scenario on `/summary` keeps the banner off when only the chart sub-query fails.
- `insights_refresh_test.go` — per-account worker no-ops with nil cache/computer/account; writes all three endpoints on success; preserves pre-seeded cache when Langfuse-out; returns non-nil error on transient failure (so River retries) but nil on all-failed (so it doesn't); partial-failure writes only succeeded endpoints; one endpoint's failure doesn't short-circuit the rest.

## Migration

None — additive boolean field on responses, new optional config field on `riverqueue.Config`, additive Redis cache (cold-start behavior identical; warm-cache path is faster).
