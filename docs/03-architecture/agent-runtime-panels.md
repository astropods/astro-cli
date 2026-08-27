# Agent detail: runtime observability panels

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

This covers the pages a user lands on to *operate and inspect an
already-deployed agent*: the Monitor, Traces, and Deployments tabs under
`:account/agents/:deploymentId`, plus the `/agents` dashboard list. It does
not cover:

- **Deploy/redeploy mechanics.** The Configure tab
  (`pages/agent-detail/AgentConfigure.tsx`) renders the same deploy form
  documented in [`blueprint-deploy-flow.md`](blueprint-deploy-flow.md) —
  see that doc for the form itself. This doc only notes how Configure fits
  into the tab bar.
- **Deployment status semantics** (what `active`/`failed`/`stopped` mean,
  who sets them, recovery behavior). That's
  [`deployment-state-machine.md`](deployment-state-machine.md). This doc
  covers the client-side plumbing that *reads* that state, not the state
  machine itself.
- **The Eval tab** (`pages/agent-detail/AgentDataset.tsx`,
  `components/agent-detail/evals/**`). Its own area, documented in
  [`traces-to-eval-dataset.md`](traces-to-eval-dataset.md).

## Tab structure

`apps/astro-client/src/routes.ts` nests five routes under
`:account/agents/:deploymentId`, rendered via `AgentTabBar.tsx`:

| Tab label | Route segment | Page |
|---|---|---|
| Monitor | `monitor` | `AgentMonitor.tsx` |
| Traces | `traces` | `AgentTraces.tsx` |
| Deployments | `deployments` | `AgentDeployments.tsx` |
| Configure | `configure` | `AgentConfigure.tsx` (documented elsewhere, see above) |
| Eval | `dataset` | `AgentDataset.tsx` (documented elsewhere, see above) |

The Eval tab only appears behind the `evals` experiment flag
(`AgentTabBar.tsx`'s `BASE_TABS` filter). The bare index route
(`AgentDetailRedirect.tsx`) redirects to `deployments` — **Deployments, not
Monitor, is the default tab** a user lands on.

There is no route or tab literally named "Pods" or "Network." The pod
topology graph is the main content of the **Deployments** tab; the network
flow graph and traffic tables are a section within the **Monitor** tab.

### Shell (`AgentDetail.tsx`)

`pages/AgentDetail.tsx` is the layout route: it loads the deployment record
server-side (`loader`), then client-side calls `useDeployment` (record),
`useDeploymentRuntime` (live workload state), `useDeploymentStatus` (coarse
status), and `useDeployments` (list, to backfill `latest_build_id` for the
Launch button's chat-eligibility check). It renders the starfield
background, `AgentIdentity` (avatar/name/overflow menu), `AgentTabBar`, the
active/paused toggle (`AgentStatusToggle`), and a Launch-to-chat button,
then hands `{ deployment, runtime, account, deploymentId }` to child tabs
via `useOutletContext` (`useAgentDetailContext`). A deleted/404'd deployment
renders a not-found state instead of crashing child tabs on a null context.

`shouldRevalidate` deliberately blocks the loader from re-running on
tab-to-tab navigation within the same deployment — TanStack Query owns
freshness after the first load, not the router loader.

## Monitor tab (`AgentMonitor.tsx`)

Three sections, two backends:

### Token usage / requests & latency — Langfuse

`useObservabilityMetrics` (`api/queries/observability.ts`) hits
`GET /observability/metrics`, handled by `GetLangfuseMetrics`
(`apps/astro-server/handlers/observability_langfuse.go`), which issues two
Langfuse Metrics API queries (`view: observations` for token/count,
`view: traces` for latency stats) filtered by `tags: deployment:<id>` and
bucketed hourly. The client aggregates those hourly buckets into local-day
bars (`aggregate-token-buckets.ts`) for `TokenUsageChart` (stacked
Recharts bar) and `RequestVolumeChart` (Recharts area); `LatencyCard` is
pure presentation, deriving weighted avg/min/p95/max from the same points.
No background polling — `LIVE_QUERY_OPTS` refetches only on mount or
range-param change.

### Network traffic — Prometheus, fed by Beyla eBPF

`useNetworkSummary`, `useNetworkFlows` (`api/queries/network.ts`) hit
`GET /network/summary` and `/network/flows`, handled by `GetNetworkSummary`
/`GetNetworkFlows` (`apps/astro-server/handlers/network.go`), which run
PromQL against a `promquery.Client` scoped by the `k8s_namespace_name` and
`service_name` labels Beyla always emits (Beyla doesn't expose arbitrary
pod labels, so those two are the only reliable join keys). This is a
metrics-store read, not Langfuse and not the DB. No polling either — flows
use a 5-minute `staleTime` ("flows tables don't need to be live"),
summary/timeseries use `staleTime: 0` (refetch on mount/range change only).

**`NetworkFlowGraph.tsx` has real graph-layout complexity**, unlike the pod
graph below: it groups outbound flows into vendor-level bubbles by
registrable domain (`destination-groups.ts`'s `groupDestinations`, using
the server-computed eTLD+1 rather than a client guess), sizes bubbles by
`scaleSqrt` on request count, and runs an actual `d3-force` simulation
(`forceX`/`forceY` pulling bubbles to their side, `forceCollide` to keep
them from overlapping) for a fixed 300 ticks synchronously — not a live
animation loop, just a one-shot layout solve. Long tails are capped at 20
bubbles per side (`truncateForDisplay`) with a synthetic "+N more" node
absorbing the remainder. `NetworkFlowsTable` and `NetworkSummaryCard` are
plain data tables/cards over the same query results.

## Traces tab (`AgentTraces.tsx`, `components/agent-detail/traces/**`)

**Entirely Langfuse-backed, not the DB.** Every hook in
`api/queries/observability.ts` used here
(`useObservabilityTracesInfinite`, `useObservabilityTraceUsers`,
`useObservabilityTraceDetail`, `useObservabilityObservationDetail`) calls a
`GET /observability/...` endpoint routed to a handler in
`apps/astro-server/handlers/observability_langfuse.go`
(`GetLangfuseTraces`, `GetLangfuseTraceUsers`, `GetLangfuseTraceDetail`,
`GetLangfuseObservationDetail`). Each handler builds a per-account
`langfuse.Client` from stored Langfuse keys and reads straight from
Langfuse; `GetLangfuseTraceDetail` even re-checks
`langfuse.HasDeploymentTag` on the returned trace as a defense-in-depth
tenancy check, which only makes sense against Langfuse's own tag-based
multi-tenant store. Astro's DB is not a data source here — the server's
role is auth/deployment resolution, response shaping, and (for
non-Langfuse-orderable queries) an in-process **criteria cache** that loads
and paginates a filtered/sorted candidate set server-side when the
request's sort or filter doesn't map onto a Langfuse-native `orderBy`; a
capped scan surfaces as the "Partial results · N candidates checked"
banner in `TracesTable`.

List pagination is a `useInfiniteQuery` over `offset`/`limit=100` pages,
windowed again client-side by "Show more" before triggering a real
`fetchNextPage`. No background polling (`staleTime: 0`, refetch on
mount/filter change only); `useObservabilityObservationDetail` uses
`staleTime: Infinity` since observation content is immutable once written,
and `useObservabilityTraceDetail` explicitly disables `placeholderData` so
navigating between traces never shows a stale different trace mid-fetch.

**The observation tree is real tree construction, not a flat indented
list.** `observation-utils.ts`'s `buildObservationTree` reconstructs
parent/child structure from Langfuse's flat observation array, promotes
orphans (any node whose declared parent isn't in the trace) to roots so
nothing silently disappears, recursively sorts children by start time, and
precomputes per-node depth/rail-connector state for `ObservationTreeNode`'s
rendering. A second, independent layer (`computeTraceBounds`/
`nodeTimespan`) computes a shared time axis across all observations to
drive the waterfall bars. Observation bodies (input/output/metadata) are
lazily fetched only when a node is selected — the tree-skeleton response
omits them on purpose (a ClickHouse-cost optimization on Langfuse's side).

`TraceDetailPanel.tsx` uses `SidePanel` (added in the "reusable SidePanel
shell" migration); `AgentConfigure.tsx` is a full-page form and never
imports `SidePanel`. `apps/astro-client/CLAUDE.md`'s side panel pattern
section already reflects this split (`TraceDetailPanel`/`PodDetailPanel`/
`ChatWorkspace` as `SidePanel` children, Configure explicitly called out as
a full page, not a panel).

## Deployments tab (`AgentDeployments.tsx`) — the pod graph

This tab is two things side by side: a live pod topology graph, and a
history sidebar. It's the one place in agent-detail that most directly
exercises the "K8s for live status, DB for what we deployed" rule, and it
does so by merging both, keyed by workload name:

```
specByName  = deployment.workloads       (DB record — what we deployed)
liveByName  = runtime.workloads          (see below — live-ish status)
workloads   = union of both name sets, spec fields as defaults, live fields overlaid
```

The **spec list is the stable source of truth for which tiles exist** — it
doesn't flicker on pause/resume/redeploy. Live data fills in status without
ever hiding a tile the spec says should exist. A runtime-only entry (e.g. a
manual ingestion Job that isn't in the normalized spec) still gets a tile
via the union.

### Where "live" data actually comes from

None of this is a per-request K8s API call from the handler. Everything
routes through DB-persisted projections that `deploycontroller` (see
[`deployment-state-machine.md`](deployment-state-machine.md)) keeps current
via its K8s informer:

| Data | Hook | Handler | Source |
|---|---|---|---|
| Per-workload live status | `useDeploymentRuntime` | `GetDeploymentRuntime` | `deployStore.GetRuntimeSnapshot` — a DB row the controller writes, not a live K8s Get. Comment: a disabled/unreachable cluster returns the last-observed snapshot, "never a 503." PVC usage is overlaid live from Prometheus for StatefulSet workloads only. |
| Coarse status badge / failure banner | `useDeploymentStatus` | `GetDeploymentStatus` | Purely DB: transitional/paused/suspended/failed statuses resolve straight from `dbDep.Status`; the `active` case reads `deployStore.GetWorkloadStatuses` (also controller-written) rather than probing K8s. No live K8s call exists in this handler. |
| K8s events (Events sub-tab) | `useDeploymentEvents` | `GetDeploymentEvents` | `deployStore.GetRuntimeSnapshot(...).Events` — the controller's humanized copy of K8s Event objects, read from DB, not a live `kubectl get events` equivalent. |
| Deployment history (sidebar) | `useDeploymentHistory` | `GetDeploymentHistory` | `deployStore.GetDeploymentHistoryByRevisions` — plain DB read, no K8s involved. |

**The one real exception:** `GetWorkloadMetrics` (Metrics sub-tab, see
below) does an actual per-request live `CoreV1().Pods(...).Get()` to read a
pod's CPU/memory resource **limits** and mounted PVC names
(`podClusterInfo`) — data the DB's deployment spec already has
(`resources.cpu`/`resources.memory`). This runs on every poll while the
Metrics sub-tab is open (every 30s at the 1h range, up to every 5 min at
7d), not just once. It fails soft (returns a zero-valued struct on any
error, degrading the chart's limit line rather than erroring), but it's a
genuine "what did we deploy" read routed through a live K8s call instead
of the DB record — worth revisiting against the stated rule.

### Pod graph layout — deliberately simple

Unlike the network graph, the pod graph's layout math is intentionally
plain. `pod-layout.ts`'s `computeColumnLayout` is a pure function: bucket
tiles into four role columns (`ingestion | knowledge | agent | others`,
from `classify.ts`), stack each column vertically by summed tile height,
lay columns left-to-right centered on the origin. `pod-edges.ts`'s
`computeRelationshipEdges` draws the agent as a hub with spokes to every
other tile, except ingestion tiles fan out to knowledge tiles instead
(ingestion feeds knowledge stores, not the agent directly). No simulation,
no iteration — same input always produces the same layout. This is a
deliberate choice: commit `a0ce190cd` replaced an earlier `d3-force`
version with this deterministic layout specifically so edges reflect real
workload wiring instead of physics-based proximity.

The actual complexity in `PodGraph.tsx` is UI plumbing around that simple
layout: continuous `ResizeObserver`-based tile measurement
(`use-tile-measurements.ts`), a hand-rolled pan/zoom engine
(`use-pan-zoom.ts` — wheel pan, ctrl/cmd+wheel zoom-toward-cursor, drag
pan, fit-to-viewport, resize-triggered reset), a canvas/vertical-list
breakpoint switch for narrow viewports, and Motion-driven enter/exit
animation sequencing.

### Pod detail panel (`PodDetailPanel.tsx`)

A `SidePanel` with five sub-tabs: General, Logs, Metrics, Events, Alerts.

- **Logs** (`PodLogsTab.tsx`): `useDeploymentLogs`/`useLastErrorLog`, backed
  by a Loki client (`lokiClient` in `GetDeploymentLogs`) — a fourth,
  distinct backend from DB/K8s/Langfuse/Prometheus. `use-container-log-errors.ts`'s
  background per-container error probe is consumed by `PodTile.tsx` only
  (its error dot in the grid view) — the detail panel itself always opens
  on General and has no error-driven banner or auto-switch; that behavior
  was removed (PR #2153) since the Logs tab already surfaces the error with
  its surrounding lines, and opening a pod no longer fetches every
  container's logs as a result.
- **Metrics** (`PodMetricsTab.tsx`): `usePodMetrics` → `GetWorkloadMetrics`,
  Prometheus/cAdvisor-backed for CPU/memory/network/filesystem/OOM/restart
  series, plus the live K8s Get for limits/PVCs noted above. Poll interval
  scales with the selected range (30s at 1h, 60s at 6h, 5 min at 24h/7d).
- **Events**: DB-cached K8s events (see table above).
- **Alerts**: `useDeploymentAlerts` — the observation alert engine, its own
  documented area ([`observation-alerts.md`](observation-alerts.md)); not
  re-covered here.

### Deployment history panel (`DeploymentHistoryPanel.tsx`)

DB-backed (`useDeploymentHistory`), plus two secondary reads:
`useAccountBlueprints` (detect a newer published build, drives the
"upgrade available" nudge) and `useGitHubStatus` (polls every 15s, only
while the deployment is GitHub-sourced, to show an in-flight build card).
Only the currently-active tile calls `useDeploymentStatus` for a live
badge; historical tiles render without one.

## Shell components (`components/agent-detail/*.tsx`, top level)

- **`AgentTabBar.tsx`** — pure nav chrome, no data fetching; collapses to a
  `Select` dropdown below 1280px.
- **`AgentIdentity.tsx`** — avatar/name header plus an overflow menu (view
  blueprint, share trading card, restart, delete). View blueprint is a
  plain `Link` navigation; the other three each open their own dialog.
- **`AgentStatusToggle.tsx`** — the active/paused switch. Reads
  `useDeploymentStatus` and `useBillingStatus`, calls
  `useStopDeployment`/`useWakeUpDeployment`, and tracks a local optimistic
  "pausing/resuming" intent until server state confirms it.
- **`AgentDeploymentMenu.tsx`** — the cross-agent/cross-account switcher
  popover. Backed by `useDeploymentsSummary` (`GET /deployments/summary`,
  DB-backed, refreshed only by mutation invalidation, no polling).
- **`PanelSection.tsx`** — a generic title/description/empty-state wrapper;
  no data fetching, not agent-detail-specific in any deep way.

## The `/agents` dashboard (`AgentDashboard.tsx`, `components/dashboard/**`)

A distinct page from agent detail: the cross-account list a user sees
before drilling into one agent. `AgentDashboard.tsx` calls
`useUserDeployments` (an infinite query over `GET /me/deployments` →
`ListVisibleDeploymentsForUserPage`, DB-backed via `deploymentstore` with a
generation-vector cache layer) for the list itself, and polls every 3s only
while some visible deployment is in a transitional status
(`pending`/`provisioning`/`deploying`/`undeploying`).

`DeployedAgentsSection.tsx` is presentational — it receives the list as
props rather than fetching. Inside it, `useDeploymentSummaryMaps.ts` wraps
`useVisibleDeploymentSummaries` (`GET /me/deployment-summaries`, batched
100 IDs per request) to overlay a per-card request/token sparkline; that
endpoint reads a Redis-backed cache (`k8scache.Cache`) keyed per
deployment, a separate rollup pipeline from the DB list itself —
consistent with the account-level rollup covered in
[`insights.md`](insights.md), not re-derived here. `useAgentFilters.ts` is
local client-side filter/sort state (status filter persisted to
`localStorage`); `DashboardToolbar.tsx` and
`DashboardAgentsEmptyState.tsx` are presentational.

## Reuse of `components/activity/**`

`components/activity/**` is owned by the account-level Insights page (see
the Insights row in `docs/README.md`), not by agent-detail. Agent-detail
reuses exactly two things from it, both narrow:

- **`TimeRangeSelector`** — the shared range-pill control, imported
  directly by `AgentMonitor.tsx` (day-range and network-direction pills)
  and `AgentTraces.tsx`, and by `PodMetricsTab.tsx` for its own range
  picker.
- **`SlackUserIdentity` / `insights-user-identity.ts`** — imported by
  `traces/TraceUserIdentity.tsx` and `traces/TracesTable.tsx` to render a
  trace's originating user consistently with how Insights renders the same
  identity.

No chart or data-fetching code is shared — the reuse is UI primitives and
one identity-formatting helper, not a shared metrics layer. `AgentDetail.tsx`
and `AgentDeployments.tsx` import nothing from `components/activity/**`.

## Real code issues found (not fixed here)

1. **`GetWorkloadMetrics`'s live K8s Get for resource limits** (see
   "Deployments tab" above) reads spec-level data (CPU/memory limits, PVC
   names) directly from K8s on every poll, instead of the DB record that
   already has it. Fails soft, but is a live per-request K8s call for data
   the deployed spec already defines — the kind of thing
   `apps/astro-client/CLAUDE.md`'s K8s-usage rule exists to avoid.
2. **Test coverage is uneven and concentrated on pure logic, not
   orchestration.** Across `pages/agent-detail/**` (excluding `AgentDataset`),
   `components/agent-detail/**` (excluding `evals/**`), and
   `components/dashboard/**`, there are 60 non-test source files and 26
   test files (43%). The gap is concentrated exactly where the real logic lives:
   `observation-utils.ts` (trace-tree construction, orphan promotion,
   waterfall geometry), `TraceDetailPanel.tsx`, `AgentTraces.tsx`,
   `PodGraph.tsx` (pan/zoom/measurement orchestration), and
   `NetworkFlowGraph.tsx` (the `d3-force` bubble layout) have no test
   file, while comparatively presentational leaf components
   (`TracesTable`, `ContentSection`, `TracePanelHeader`, `classify.ts`,
   `pod-layout.ts`, `pod-edges.ts`, `destination-groups.ts`) are tested.
   The pure-function modules being tested and the stateful/visual ones not
   being tested is a consistent pattern, not random gaps.

## Recent churn

The pod graph, network graph, and trace detail panel are all
high-churn areas:

- `a0ce190cd` — replaced the pod graph's original `d3-force` layout with
  the current deterministic columnar one.
- `3521594f9` — added the pod graph's pan/zoom canvas.
- `560d17420` — added the network flow graph (the `d3-force` bubble
  layout) to the Monitor tab.
- `52005feef` — introduced the shared `SidePanel` shell and migrated the
  trace detail panel onto it.
- `15064258d` — most recent trace-related fix, a Langfuse v4 migration fix
  for trace cost display.
- Several Deployments-tab fixes cluster around stuck/failed-deploy UX
  (`d1cd09409`, `09cfe453f`, `969c1bfc8`, `c4c397f93`) rather than the pod
  graph's layout itself.
