## Summary

Improves the trace viewer in the Monitor tab with rich text rendering for input/output content and a new slide-out detail panel for inspecting individual traces. Also adds a local development mock for observability data so the monitor tab works without a live backend.

## Design

### Trace detail panel

Replaces the inline row expansion with a persistent right panel, reusing the same slide-out pattern as the Configure panel (`ActiveDetailView` → 420px sticky right column with a width transition). Clicking a trace row opens the panel; clicking a different row swaps the content. Opening Configure closes the trace panel and vice versa.

The panel header follows the `ConfigurePanel` shell exactly — `QueueListIcon` + "Traces" label + up/down chevron `Button` icon buttons for keyboard-free navigation through the trace list + close button. Navigation is driven by `visibleTraces` in `MonitorTab`, passed up to `ActiveDetailView` via an `onVisibleTracesChange` callback and `useMemo`-stabilised to avoid infinite render loops.

On compact viewports the panel renders full-page inside the content area, matching the existing Configure compact behaviour.

**Metadata strip** uses a two-row layout: timestamp + status badge on the first row, latency + token count on the second. Both rows use the mono font and foreground/muted-foreground token hierarchy.

**Input/Output tabs** — tab underline hugs the text width. A `Button ghost icon` copy button is positioned `absolute top-3 right-3` over the content area so it doesn't affect layout.

### Rich text rendering

Input and output fields now render via `StyledMarkdown` (the same component used for agent READMEs) instead of displaying raw strings. This renders markdown headings, lists, tables, code blocks, and inline code with the existing prose styles.

`TraceRow` mapping in `MonitorTab` was already normalising non-string values to JSON — those render correctly as fenced code blocks.

### Shared utilities

`TraceStatus`, `TraceRow`, `TRACE_STATUS_STYLE`, and `formatLatencyMs` are now exported from `MonitorTab` so `TraceDetailPanel` can import them directly without duplication.

### Mock observability data

Adds MSW browser mocking (`src/mocks/`) and a Vite dev server middleware (`mockObservabilityPlugin` in `vite.config.ts`) that intercepts the three observability endpoints when `VITE_MOCK_API=true`. The Vite middleware runs server-side and is more reliable than the service worker alone — it intercepts before the proxy forwards to the real backend. Mock traces include realistic markdown content across all five output variations to exercise rich text rendering locally.

## Migration

No migration required. `VITE_MOCK_API=true` is set in `.env` by default for local dev. Hard-refresh the browser after pulling if the monitor tab shows a stale service worker error.
