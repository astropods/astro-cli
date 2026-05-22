# Network Traffic on the agent Monitor page

## Summary

The per-deployment network metrics API landed in #1123 but had no UI surface. This PR wires those endpoints into the existing agent Monitor tab so users can see, at a glance, what HTTP and database traffic each agent is making and how it's faring.

## Design

**Placement on Monitor, not a new tab.** Network traffic lives as a fourth section on `AgentMonitor` (peer to Token Usage / Requests & Latency / Traces). Keeping observability consolidated on one page beats forcing users to switch tabs to answer "is my agent healthy?" The page already orders sections from highest-level (token spend) to lowest-level (individual traces); network traffic slots in between system-level requests and the trace records that explain them.

**Two components, one section:**
- `NetworkSummaryCard` — three cards (Inbound / Outbound / Database) mirroring the `LatencyCard` shape: hero metric (request count) + sub-metrics. Em-dash empty state for zero traffic, support for a custom empty message (e.g. when Bun outbound isn't instrumented).
- `NetworkFlowsTable` — direction-pill-switched table of top peers per direction. Three columns: peer, requests, responses.

**Responses column as a visual bar, not status-code columns.** Earlier iterations had separate `Errors %`, `2xx`, `4xx`, `5xx` columns and competed for attention while saying redundant things. The final cell renders a stacked horizontal bar (success / 4xx / 5xx, segment widths proportional to counts) plus a `99.5% ok` label in tabular-nums. Hover surfaces exact counts via tooltip. The bar instantly answers "is this working" — rows with visible red slivers pop. Hit-box covers the entire cell, not just the bar, so the tooltip is easy to land on.

**Shared time window.** The whole Monitor page uses one 7D/14D/30D selector. Network hooks subscribe to the same window state — no per-section pickers — so flipping the range refreshes every panel together.

**Direction pill reuses `TimeRangeSelector`.** Its `{ key, label }` ranges prop is structurally identical to what direction switching needs; passing `NETWORK_DIRECTIONS` and a distinct `layoutId` gives the same animated-pill UX without a second component.

**Cache key extension.** `networkKeys.flows` now takes `limit` as a fifth arg so future paginated flows tables can address different page sizes without colliding on cache identity. The default-50 server cap is unchanged.

**Storybook coverage.** Both components ship stories under `Features/Agent Detail/`, including representative high-traffic, high-error, empty, loading, and many-rows variants. The summary card has a `ThreeUp` story that renders the exact Monitor-page layout for design review without booting the app.

## Migration

None — additive. Existing Monitor sections are untouched; the new section appears between Requests & Latency and Traces.
