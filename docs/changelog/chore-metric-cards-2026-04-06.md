# MetricCard trend indicator opt-in

## Summary

The `MetricCard` component previously always rendered a trend indicator row, showing `— —` even on cards that will never have trend data (e.g. the all-time totals on the dashboard). This added visual noise and implied a comparison that doesn't exist.

## Design

A `showTrend` prop (default `false`) controls whether the trend indicator is rendered. Cards opt in explicitly rather than suppressing it.

- `DashboardStats` cards omit `showTrend` — the indicator is hidden by default.
- `HeadlineMetrics` passes `showTrend` — dashes render while data is null/loading, arrows and percentages render once trend data is available.

The `trendLoading` skeleton path is also gated on `showTrend`, so cards without trend data never show a skeleton row either.

## Migration

Any `MetricCard` usage that relied on the trend indicator rendering by default must now pass `showTrend` explicitly.
