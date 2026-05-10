# Trace detail panel + per-bucket latency

## Summary

The monitor page's trace side-panel was a flat input/output dump that couldn't drill into the underlying observation tree, and the latency card was rendering zeros because the buckets payload never carried real latency. This PR ships a Langfuse-style trace viewer (overview + observation tree with timing waterfall) and fills in the missing latency data on the buckets endpoint.

## Design

### Per-bucket latency

`GetLangfuseMetrics` now issues a second `MetricsQuery` against the **traces** view for `latency` (avg / p95 / min / max), keyed by the same time-dimension as the tokens query, and merges results by timestamp. Trace-view latency is preferred over observation-view latency — the latter would understate request duration by counting each sub-span in isolation. If the second query fails the handler logs and serves zeros instead of failing the metrics call.

`MetricsBucket` gains `p95_latency_ms`, `min_latency_ms`, `max_latency_ms`. The `LatencyCard` consumes these to show **Min | P95 | Max** alongside the avg hero metric, replacing the old per-day-avg "Range" tile that always read `1.6s – 1.6s` when only one day had data. An empty state ("No requests in this range") replaces the misleading `0ms` display when no traces exist.

### Trace detail endpoint

```
GET /api/v1/deployments/:id/observability/traces/:traceId
→ { trace, observations: [...], scores: [...] }
```

Wraps Langfuse's `/api/public/traces/{traceId}` (which inlines observations + scores). The handler authorizes via the existing `resolveLangfuseContext` helper and additionally checks the trace's `deployment:{id}` tag — defense in depth so a trace ID from a different deployment in the same Langfuse project can't leak through.

The response is projected into a stable shape (`projectTrace` / `projectObservations` / `projectScores`) so the frontend doesn't drift if Langfuse's schema changes. Observation `latency` is multiplied ×1000 because Langfuse reports observation duration in seconds (different from the metrics API, which is ms — the inconsistency tripped up the first iteration).

### Trace detail panel — modular rebuild

The panel was rewritten as a composition of single-purpose components under `agent-detail/traces/detail/`:

```
TraceDetailPanel
├── TracePanelHeader        — title, prev/next, maximize, close
├── TraceMetaGrid           — Status / Latency / Cost / Tokens
├── TraceTabs               — Overview | Tree
├── TraceOverviewTab
│   ├── ContentSection      — collapsible body, copy
│   │   └── JsonView        — Prism JSON via react-syntax-highlighter
│   ├── MetadataList        — key/value rows, baseline-aligned
│   ├── TagsRow             — tag pills (deployment:* hidden)
│   └── ScoresList          — eval scores
├── ObservationTree
│   └── ObservationTreeNode — two-row layout, rails, waterfall
└── ObservationDetail       — selected-observation pane
```

`observation-utils.ts` builds the tree from the flat list, decorates each node with `depth` / `isLast` / `ancestorsLast[]` so the connector rails draw correctly, and provides `computeTraceBounds` + `nodeTimespan` for the waterfall geometry.

### Tree visual model

Each row has a two-line layout (name + icon on top, latency / token flow / cost below) with type-specific icons (`ArrowLeftRight` for spans, `Sparkles` for generations, `Zap` for events). When the panel is expanded a 160px waterfall column appears on the right; bars are pinned to the name-line height (not the row middle) so multi-line rows still share a horizontal axis. Bars are uniformly indigo, coral only on errored observations — an earlier heat-gradient experiment was too distracting. When an observation's name already contains its model the duplicate is suppressed.

### Panel sizing

Default width 28rem → 40rem, with a maximize button in the header that promotes the panel to full-screen overlay (using the same code path that already kicks in on narrow viewports). Tree tab in expanded mode renders side-by-side: tree on the left, observation detail on the right. Narrow mode keeps the master/detail flow with a back button.

## Migration

None. New endpoint, additive bucket fields, additive UI.

For local development, slow deploys (10–11s) can hit the default 10s `WRITE_TIMEOUT` and surface as a Vite proxy `socket hang up`. If you see this, set `WRITE_TIMEOUT=120s` (and `READ_TIMEOUT=120s`) in your local astro-server env. Defaults remain unchanged so production behavior is unaffected.
