# Insights reads stored usage, and the cached path is gone

## Summary

Insights had two read paths. The default one re-aggregated 90 days of Langfuse data per account every six hours, stored the rendered page in Redis, and served that; the rollup-backed one read a durable daily fact table and shipped behind a "Faster Insights" experiment so the two could be compared on the same account.

The comparison is done. The rollup path now serves every request, and everything the cached path needed is deleted: the Redis layer, the two refresh workers, the six-hourly periodic job, the dev-tool fold, and the summary-computer seam that existed only to let a River worker call the handlers' aggregate code.

## Design

### One endpoint, no version in the URL

The rollup handler took over `GET /api/v1/accounts/:account/insights` and the `/api/v2` group is gone. The version existed so both paths could be called for the same account during verification; keeping it afterwards would make every future client carry a wart that describes a migration nobody remembers. The wire contract is unchanged, so a client bundle cached from before this change keeps working.

The client-side experiment is gone with it. `useAccountInsights` no longer branches, and the query key no longer carries a version.

### What the read path costs now

A request is a handful of SQL aggregates against `insights_usage_daily`, plus the DB reads that supply row identity. Nothing on the page queries Langfuse except the Models view, which still reads the summary endpoint.

The six-hourly warm-cache pipeline is deleted rather than repointed. Its only purpose was pre-warming the three account-level observability aggregates for this page; with the page off them, keeping it would mean re-aggregating 90 days per account four times a day for endpoints nobody calls with the default params. Those endpoints (`/observability/summary`, `/observability/deployments-summary`, `/observability/users-summary`) now always compute live, which is already what they did for every period-bounded call the client makes.

Removed with it: `internal/insightscache`, `InsightsRefreshWorker` and `InsightsRefreshAccountWorker`, the `riverqueue.InsightsSummaryComputer` seam and its `handlers` implementation, and the insights entries in both cache-invalidation paths (admin gRPC and `accountcache`). `ErrAllLangfuseCallsFailed` moves back to `handlers`, where its only remaining callers live: it lived in `insightscache` so the refresh worker could `errors.Is` on it without closing a `handlers→riverqueue→handlers` cycle, and that worker is gone.

Two River job kinds disappear, `insights.refresh` and `insights.refresh_account`. Rows of those kinds left in the queue at deploy time have no worker and will sit until cancelled; nothing else waits on them.

### The fold is gone, not ported

The cached path folded dev-tool spend into each surface separately: chart series, stat-card totals, series labels, a synthetic agents row, and a per-developer roll-up into the People rows. It had to, because dev-tool spend arrived from a pipeline the base data knew nothing about. Every surface therefore needed its own compensation, and a surface that missed one contradicted the others.

The fact table carries both sources, so there is nothing to compensate. Dev-tool spend reaches the cards, chart and agents table as a synthetic deployment entry, and the People surfaces from the actor grain. `buildInsightsViewWithParams` now takes the list of present sources for the Sources filter instead of a fold descriptor, and `hide_sources` is applied once as a `WHERE` clause instead of again in Go. Deleted: `insights_devtool_fold.go`, `computeDevtoolForInsights`, `devtoolSourceFor`, and the `DevtoolSource`/`DevtoolRange` types that only the fold read.

`metrics_unavailable` is dropped from `InsightsResponse`, along with the banner it drove. It reported "every Langfuse call for this page failed", a state the read path can no longer be in: Postgres either answers or the request fails and the client renders its error state.

### What users see change

- **Tables respect the range chip.** The client always sends `days`, so the agents and People tables scope to the selected window instead of showing 90 days under a 7-day chart.
- **The page ends at the last complete day.** Windows end on the roll-up watermark, and the footer names that day ("Showing everything through Aug 16"). Insights is a daily report: today's spend is deliberately not shown, because serving a partial day means a live upstream query on every request.
- **Change % survives dev-tool spend.** The fold discarded it, because it was computed from agent spend alone.
- **Accounts past 100 deployments get complete tables.** The old read path capped its per-deployment fan-out at 100.
- **`p95_latency_ms` stays empty** on the agents table, which shows last-used activity instead. Percentiles need fixed-boundary histograms at ingest; see [insights-rollup-spec.md](../../01-spec/insights-rollup-spec.md).

## Migration

No user action, no schema change, no config change. The fact table and its workers already run in production, so the swap is a read-path change against data that is already there.

An account with nothing rolled up yet renders zeros with no coverage day until its first roll-up lands. The roll-up backfills 90 days on first run per account.

Operators lose one lever: the admin cache-invalidation RPCs no longer bust anything for Insights, because the page no longer reads a cache. Stale-looking Insights data is now a roll-up question, and `as_of` on the response is where to look.
