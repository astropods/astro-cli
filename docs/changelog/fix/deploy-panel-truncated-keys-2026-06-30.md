## Summary

Chart cards in the pod detail panel (deploy panel) were clipping content. The Network and Filesystem (Read/Write) cards cut off their legend keys at the bottom, and the topmost Y-axis tick and rightmost X-axis time tick were also being trimmed against the chart's edges. This made the charts harder to read and looked unpolished.

## Design

`ChartCard` previously wrapped all of its `children` in a fixed `h-48` box. Each chart passes a `ResponsiveContainer` at `height="100%"` (which fills the full 192px) followed by its legend rows — so the legend overflowed the fixed-height box and was clipped. The fix scopes the `h-48` height to the chart plot area only: each chart now wraps its `ResponsiveContainer` in its own `h-48` div, and the legend flows naturally below it inside the card. Loading and empty placeholders keep the `h-48` sizing.

Separately, the axis ticks were clipping against the SVG bounds because the chart margins were too tight for the two-line axis labels. The `AreaChart` margins were widened from `{ top: 8, right: 8 }` to `{ top: 16, right: 20 }` so the topmost Y tick and the last X time tick have room to render fully.

## Migration

No action required.
