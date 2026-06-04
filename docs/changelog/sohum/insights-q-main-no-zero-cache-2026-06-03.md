# Insights: Q_main day granularity (was hour), retire all-time toggle, defensive partial-failure guard

## Summary

On the Insights People tab, several accounts were rendering users with $0 cost / 0 requests / 0 tokens — even though those users had real traffic on the Agents tab. The visible bug was a partial-failure cache poisoning, but the underlying cause was that we were asking ClickHouse to scan and group **per-userId per-hour buckets over five years of trace history** on every refresh. Q_main was timing out at the 30s upstream cap for any account with non-trivial traffic; the per-account refresh job was caching the zero-result response; the cache TTL kept those zeros on screen until the next refresh succeeded.

This PR removes the cardinality problem at its source.

## Design

Three coordinated changes — server, defensive guard, and UI cleanup:

**1. Q_main day granularity (was hour).** `ComputeUsersSummary`'s primary query carried `TimeDimension: hour`, producing up to ~43,800 hour buckets × userIds × tag-filtered traces over a 5y window. The only field we ever read out of per-bucket timestamps is `last_seen` (max non-zero bucket), and day-resolution is plenty for that — the People row renders "last seen 3 days ago"-style relative time. Switching to day granularity drops Q_main's bucket count by ~24× for the same product behavior and is the actual fix for the ClickHouse timeout. The 5-year all-time window is preserved (the Agents and People tables are intentionally lifetime views — see #3).

**2. Defensive partial-failure guard.** `ComputeUsersSummary` previously emitted `ErrAllLangfuseCallsFailed` only when *every* sub-query failed. When only Q_main failed (the heavy one), the response fell through with users discovered via Q_tags but all-zero metrics, and the 6h refresh worker cached those zeros for the entire next window. Replaced the `lfAttempts`/`lfFailures` tally with a focused `mainQueryFailed` bool: Q_main failure alone now signals all-failed, the handler returns `metrics_unavailable: true` with an empty list, and the worker skips the write — preserving the prior cache. Q_tags failure alone is non-fatal (real metrics still land, `agents_used` is empty for that refresh). This is belt-and-suspenders for #1: if a future regression makes Q_main slow again, the cache won't get poisoned with zeros.

**3. UI: "All time" toggle retired, "90D" added.** `ActivityRange` is now `7d | 14d | 30d | 90d`. The range toggle drives the headliner KPIs and charts (sliced client-side from the all-time payload); the Agents and People tables remain unsliced and continue to show lifetime totals — they were never tied to the toggle. Stale `?range=all` deep-links fall through to the 30d default. Dead `range === "all"` branches removed from `sliceDeploymentsResponseByRange`, `sliceUsersByRange`, and `useActiveSpendSeries`; the `bounded` ternary in `sliceUsersByRange` collapses to a single unconditional path. Change-pct on the StatCards works for every range, including 90d (its prior 90d sits inside the all-time payload).

## Migration

Existing cached entries (from the hourly era) age out under the 7-day TTL or via manual `InvalidateAccountCaches` from queen. Response shape is unchanged. Stale `?range=all` URLs silently land on 30d.
