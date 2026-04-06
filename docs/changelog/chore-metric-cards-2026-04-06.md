# MetricCard and ComputeUsageCard layout improvements

## Summary

Two related changes to the dashboard metric cards: a `showTrend` opt-out for cards that will never show trend data, and layout improvements to the `UsageCard` (total compute) to align with the other metric cards visually.

## Design

### Trend indicator opt-out

A `showTrend` prop (default `true`) controls whether the trend indicator is rendered. Cards that will never have trend data pass `showTrend={false}` to opt out.

- `DashboardStats` passes `showTrend={false}` — the indicator is hidden on all-time total cards.
- `HeadlineMetrics` relies on the default — dashes render while data is null/loading, arrows and percentages render once trend data is available.

The `trendLoading` skeleton and the label-to-value spacing (`mb-4` vs `mb-2`) are both gated on `showTrend`, so cards without a trend row have consistent vertical rhythm with those that do.

### UsageCard layout

The progress bar in the total compute card moved inline to the right of the value/quota numbers. The "Request increase" button is absolutely positioned in the top-right corner so it doesn't affect the label's layout flow. Label spacing and value alignment now match the other metric cards.

## Migration

No changes required for existing usages — the trend indicator still renders by default. Pass `showTrend={false}` to hide it on cards that will never display trend data.
