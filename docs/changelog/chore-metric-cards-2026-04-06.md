# MetricCard trend indicator opt-in

## Summary

The `MetricCard` component previously always rendered a trend indicator row, showing `— —` even on cards that will never have trend data (e.g. the all-time totals on the dashboard). This added visual noise and implied a comparison that doesn't exist.

## Design

A `showTrend` prop (default `true`) controls whether the trend indicator is rendered. Cards that will never have trend data pass `showTrend={false}` to opt out.

- `DashboardStats` passes `showTrend={false}` — the indicator is hidden on all-time total cards.
- `HeadlineMetrics` relies on the default — dashes render while data is null/loading, arrows and percentages render once trend data is available.

The `trendLoading` skeleton path is also gated on `showTrend`, so cards without trend data never show a skeleton row either.

## Migration

No changes required for existing usages — the trend indicator still renders by default. Pass `showTrend={false}` to hide it on cards that will never display trend data.
