# Always show monitor charts with zeroed values instead of empty states

## Summary

The agent detail monitor page previously showed text empty states ("No token usage data yet.", "No request data yet.", "No latency data yet.") when the selected time window had no metrics. This made it unclear what the page would look like with data and felt like a broken state. Charts now always render with their axes, gridlines, and legends visible — just with zero values — so the page structure is always apparent.

## Design

The three chart components (`TokenUsageChart`, `RequestVolumeChart`, `LatencyCard`) each had a `hasData` / `!stats` guard that swapped the chart for a centered text placeholder. Those guards are removed so the chart always renders.

This works cleanly because the aggregation layer (`aggregate-token-buckets.ts`) already pads the full time range with zero-valued entries via `dayKeysForRange`, so recharts receives a complete data array regardless of whether the API returned any buckets.

For the `LatencyCard` (which is a stat display, not a chart), the metrics fall back to `0ms` for Avg Latency, P95, and Range when no request data exists.

## Migration

No migration required.
