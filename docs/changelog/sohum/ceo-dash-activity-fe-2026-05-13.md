## Summary

Adds a new Activity page (`/insights`) that gives account owners a consolidated view of LLM cost and usage across all deployed agents for a selectable time window (7d / 14d / 30d / all-time). This closes the loop on the `blueprints-summary` backend endpoint landed in the prior PR.

## Design

**Data layer** — Two TanStack Query hooks (`useAccountActivitySummary`, `useBlueprintsSummary`) call the existing observability endpoints with `?from=&to=` params derived from the selected range. Range state lives in the URL (`?range=30d` default) so links are shareable and the selection survives navigation. Both hooks use `staleTime: 5min` and `keepPreviousData` to avoid layout thrash when switching ranges.

**Layout** — `PageHeader` carries the range label and a `TimeRangeSelector` (animated pill via `motion/react layoutId`, mirroring the AgentMonitor pattern). Below that: stat cards row (collapses to 1 column at narrow widths), a cost-over-time chart, then the top-spenders table.

**Empty / loading states** — While queries are in flight, each widget renders its own skeleton (animated pulse bars for the charts, ghost table rows, grey bars for stat cards). Once both queries settle with zero data, the charts area is replaced by an `EmptyState` card pointing users to `/agents`. Stat cards remain visible in both states.

**Charts** — `CostOverTimeChart` uses a Recharts stacked `BarChart` (bar for fixed ranges, line for all-time) keyed by model. Model colors are assigned deterministically from a fixed CSS-var palette (`MODEL_PALETTE` in `model-colors.ts`) so colors stay consistent across the chart and table for the same model. `maxBarSize={56}` + `barCategoryGap="20%"` cap bar width regardless of data density.

**Top-spenders table** — Sortable on any column (default: cost desc). Shows blueprint name + top-model hint, requests, total cost, cost/req, tokens/req, P95 latency. Ghost rows pulse while loading.

**Agent filtering** — Inline combobox trigger (searchable input with pill tags, no separate search dropdown). Selected agents are used to filter both the stat cards (re-computed from blueprint-level data) and the chart series.

**Date alignment** — All date range computation is UTC-anchored. `buildPeriodParams` snaps `from` to UTC midnight of `today − (days−1)` and `to` to UTC end-of-day, so the API, chart X-axis (`dayKeysForRange` uses `getUTCDate`), and sparkline tooltips (`T00:00:00Z` + `timeZone: "UTC"`) all show identical date ranges.

**Real data** — MSW browser worker removed from `entry.client.tsx`; `public/mockServiceWorker.js` deleted. All API calls now flow through the Vite proxy to the real backend.

## Migration

No migration required — the route is additive and the endpoints it calls already exist.
