# Fix dev-tool Insights vanishing for recent-only usage

## Summary

Dev-tool usage (Claude Code) disappeared from the Insights **agents** and **people** tables and the **Sources** filter — and its chart series lost its brand color, rendering with the generic model palette — for any account whose usage was only recent (e.g. all from today). Aggregate spend was also silently undercounted: every account's synthetic agent row omitted the current day.

Root cause: the tables, the Sources filter, and branding all derive from the **widest** computed range (90d), and the per-source presence/totals were the **sum of a daily-bucket range query** (`increase(m[1d])` at a 24h step). VictoriaMetrics drops the current (partial) day from that range query for wide windows, so at 90d an account with only today's usage produced no buckets → the source was omitted entirely → empty tables, empty `devtool_sources`, and a chart series the client couldn't recognize as a branded source. An account with older usage (a few days back) survived only because those complete days kept the widest window non-empty.

## Design

**Totals and presence come from an instant window query.** A source's `Totals` and its presence now use an instant `sum(increase(m[Nd]))` over the whole window, which captures the current day; the per-day range query feeds **only** the chart's `spend_by_day` series. This mirrors the per-developer path (which already used an instant query and was therefore correct), so the agent-row total and the per-developer total now reconcile instead of disagreeing.

**Today's chart bucket is overlaid explicitly.** Because the daily range query still drops the current day at wide windows, today's spend is queried directly — an instant `increase()` since UTC midnight, computed once per source (it's range-independent) — and merged into the per-day series, keyed with the same UTC `YYYY-MM-DD` the agent-spend chart uses. The merge keeps the larger of that value and any existing bucket, so the dev-tool and agent series stay day-aligned and the chart never falls below the stat-card total.

**Day bucketing stays UTC**, matching the existing agent-spend chart. Totals and the current-day bucket are evaluated at query time, so the latest data always appears regardless of the viewer's timezone; only the day-boundary labeling is UTC (shared by both series).

## Migration

None. Behavior-only fix to the Insights view model; no API, schema, or config changes.
