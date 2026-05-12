# CEO Dashboard (Activity) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-05-11

---

## Abstract

The Activity tab in `astro-client` provides an account-scoped view of agent usage, cost, and model consumption over time. All data is sourced from Langfuse via the existing `langfuse.Client`. The dashboard is time-bounded by a selector (7D / 14D / 30D / All) and shows % change vs the prior period of equal length. All aggregation and computation happens on the server — the frontend renders only.

---

## Problem Statement

Accounts have no single place to understand how their agents are performing in aggregate — what they're spending, which models are driving cost, and which blueprints are the top spenders. The existing observability endpoints expose per-deployment metrics but no account-wide rollup, no cost data, and no blueprint-level aggregation.

---

## Goals

- **G1:** Stat cards — Cost, Requests, Tokens Consumed, Active Agents (4 cards) — time-bounded with % change vs prior period and daily average. % change is hidden when "All" is selected (no prior period exists).
- **G2:** Cost over time — stacked bar chart by model, daily granularity.
- **G3:** Cost by model — donut chart showing share of total cost per model.
- **G4:** Top spenders — per-blueprint cost table, sorted by spend descending.
- **G5:** Time range selector (7D / 14D / 30D / All) drives all widgets.

## Non-Goals

- Helpfulness / reliability scores (deferred).
- Cross-account platform-wide aggregation (internal admin use case, separate feature).
- Per-deployment drilldown within a blueprint (follow-on).
- Error rate (Langfuse doesn't expose error status in the trace list; deferred).
- Account-wide P95 latency over time chart (builder dashboard concern, not admin dashboard).

---

## No Backfill Required

This feature adds no new database tables. All data is read from the existing `deployments` table (already populated) and Langfuse (traces already flowing since the Langfuse integration shipped). No migration, no prod data risk.

---

## Data Model

Traces in Langfuse are tagged `deployment:{id}` by the astro-collector processor. Each account has its own Langfuse project. Blueprints (`agents` table) map to deployments via `deployments.agent_name`. In practice, one blueprint has one deployment per account.

Blueprint-level cost = sum of `Trace.totalCost` across all deployments sharing an `agent_name` within the account.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        astro-client                             │
│                       (Activity Page)                           │
│                                                                 │
│   useAccountActivitySummary()    useBlueprintsSummary()         │
└──────────────┬──────────────────────────┬───────────────────────┘
               │                          │
               ▼                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                        astro-server                             │
│                  observability_langfuse.go                      │
│                                                                 │
│  GET /observability/summary      GET /observability/            │
│  ?from=&to=                      blueprints-summary?from=&to=   │
│                                                                 │
│  Parallel calls via errgroup:    1. Query deployments table     │
│  • GetDailyMetrics(current)         GROUP BY agent_name         │
│  • GetDailyMetrics(prior)        2. Fan-out per deployment:     │
│                                     GetMetrics(tag=deploy:{id}) │
│                                  3. Sum per agent_name group    │
│                                  Returns sorted by cost desc    │
│  Returns pre-aggregated:                                        │
│  • stat card values + % change                                  │
│  • cost_over_time[]                                             │
│  • cost_by_model[] with %                                       │
└──────────────┬──────────────────────────┬───────────────────────┘
               │                          │
               ▼                          ▼
┌──────────────────────────┐   ┌─────────────────────────────────┐
│   Langfuse REST API      │   │        Postgres                 │
│  (per-account project)   │   │    (deployments table)          │
│                          │   │                                 │
│  /api/public/metrics/    │   │  SELECT * FROM deployments      │
│    daily                 │   │  WHERE account_id = ?           │
│  /api/public/metrics     │   │                                 │
└──────────────────────────┘   └─────────────────────────────────┘
```

---

## Frontend Mockup

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Monitor                                          [7D] [14D] [30D] [All]    │
│  Last 30 days · Apr 6 — May 5                                               │
├──────────────┬──────────────────┬──────────────────┬────────────────────────┤
│ ACTIVE AGENTS│  COST        ▲18%│  REQUESTS    ▲12%│  TOKENS          ▲22% │
│  24          │  $12,847         │  14.2M            │  284M                  │
│  ▬▬▬▬▬▬▬▬   │  $427 daily avg  │  473K daily avg   │  9.5M daily avg        │
├──────────────────────────────────────┬──────────────────────────────────────┤
│  Cost over time                      │  Cost by model                       │
│  Stacked by model · daily            │  Share of $12,847                    │
│                                      │                                      │
│  ▂▃▄▅▆▇█  (stacked bars)            │     ╭───╮   ■ claude-sonnet-4  41%  │
│  ▁▂▂▃▃▄▅  (one color per model)     │    ╱     ╲  ■ claude-opus-4-7  26%  │
│                                      │   │   4   │ ■ claude-haiku    21%  │
│  Apr 6          Apr 22       May 5   │    ╲     ╱  ■ other           12%  │
│                                      │     ╰───╯                           │
├──────────────────────────────────────┴──────────────────────────────────────┤
│  Top Spenders                                                               │
│                                                                             │
│  BLUEPRINT          REQUESTS   COST ↓    COST/REQ   TOKENS    P95          │
│  ─────────────────────────────────────────────────────────────────────────  │
│  Invoice Parser     723K       $2,460    $0.0034    28.1M     8.1s         │
│  Support Triage     4.67M      $1,892    $0.0011    118.2M    3.6s         │
│  Daily Digest       580K       $522      $0.0009    12.4M     2.8s         │
│  Lead Enricher      8.21M      $448      $0.00021   102.6M    1.1s         │
│  Weekly Recap       184K       $33       $0.00018   8.3M      0.9s         │
```

---

## Design

### Time range

The selector emits `from`/`to` ISO timestamps passed as query params to both endpoints. "All" omits both. % change is computed server-side — the server fetches the current period and the prior period of equal length, computes deltas, and returns a pre-computed `change` object. The client never computes percentages.

When "All" is selected (`from`/`to` absent), the server omits the `change` key entirely. The client treats a missing `change` as "no comparison available" and hides all % change badges.

### Aggregation principle

All computation happens on the server. The client receives fully-shaped response objects and renders them directly — no summing, no grouping, no percentage calculation on the frontend.

---

## Server Changes

### 1. Patch `GET /api/v1/accounts/:account/observability/summary`

**File:** `apps/astro-server/handlers/observability_langfuse.go`

Add query params: `from`, `to` (ISO timestamps).

The handler makes two parallel Langfuse calls via `errgroup`:
- `GetDailyMetrics(from, to)` — current period (cost + tokens + model breakdown)
- `GetDailyMetrics(prior_from, prior_to)` — prior period for % change

Extended response shape:

```
{
  "period": { "start": string, "end": string, "days": int },
  "totals": {
    "cost_usd": float,
    "requests": int,
    "input_tokens": int,
    "output_tokens": int,
    "active_agents": int
  },
  "daily_avg": {
    "cost_usd": float,
    "requests": int,
    "tokens": int
  },
  "change": {                       // omitted when from/to are absent ("All")
    "cost_pct": float | null,       // null when prior period value was 0
    "requests_pct": float | null,
    "tokens_pct": float | null
    // active_agents intentionally excluded: it's a snapshot count, not a
    // flow metric, so period-over-period comparison isn't meaningful here
  },
  "cost_over_time": [
    {
      "date": "2026-05-01",
      "models": [{ "model": "claude-sonnet-4", "cost_usd": 8.40 }]
    }
  ],
  "cost_by_model": [
    { "model": "claude-sonnet-4", "cost_usd": 5267.00, "cost_pct": 41 }
  ]
}
```

`cost_over_time` and `cost_by_model` are both derived from `DailyMetric.Usage[]` — computed in Go, returned as two separate shaped arrays.

`GetDailyMetrics` must paginate through all pages (`Meta.Page * Meta.Limit < Meta.TotalItems`) — a single request silently truncates results for long time ranges. For the "All" time range (`from` absent), use the account's earliest deployment `created_at` as the floor to bound the query; without a floor, Langfuse may return 180+ days of entries and page sizes become unpredictable. **PR 1 must add pagination to `GetDailyMetrics` in the Langfuse client layer** (`apps/astro-server/internal/langfuse/client.go`) — the current implementation makes a single request with no `page`/`limit` params and no loop. This is new work on the client, not just the handler.

`active_agents`: count distinct `agent_name` from the `deployments` table where `account_id = ?` and the deployment was live during the period (`created_at <= to` AND (`undeployed_at IS NULL OR undeployed_at >= from`)). The schema uses `undeployed_at` (nullable timestamp) — there is no `deleted_at` column. This is a pure Postgres query — no Langfuse fan-out required. "Active" means deployed during the period, not necessarily that it received traffic.

### 2. New `GET /api/v1/accounts/:account/observability/blueprints-summary`

**File:** `apps/astro-server/handlers/observability_langfuse.go`

Query params: `from`, `to`.

Logic:
1. Fetch all deployments for `account_id` from `deploymentStore`.
2. Group by `agent_name` → one group per blueprint.
3. Fan out via `errgroup`: `GetMetrics` with tag `deployment:{id}` per deployment — cost, tokens, requests, P95.
4. Aggregate per group: sum cost/tokens/requests; P95 = max across deployments (conservative approximation — not the true combined-distribution P95, but acceptable because in practice one blueprint maps to one deployment per account); compute `cost_per_request`, `tok_per_request` in Go.
5. `top_model` = highest-cost model across all deployments in the group — merge the `Usage[]` arrays from every deployment's `GetMetrics` response, sum cost per model, take the max.
6. Sort by `cost_usd` descending.

Response:

```
{
  "blueprints": [
    {
      "agent_name": string,
      "requests": int,
      "cost_usd": float,
      "cost_per_request": float,
      "input_tokens": int,
      "output_tokens": int,
      "tok_per_request": float,
      "p95_latency_ms": int,
      "top_model": string
    }
  ],
  "period": { "start": string, "end": string }
}
```

`errgroup.SetLimit(10)` with 30s timeout. Missing Langfuse data for a deployment is treated as zero — the blueprint row still appears.

Zero-value guards: when `requests == 0`, set `cost_per_request = 0` and `tok_per_request = 0` (not Go's `+Inf` from float64 division by zero, which `encoding/json` serializes as `null`). Similarly, `change_pct` fields must be `null` — not computed — when the prior period value was 0, since `(current - 0) / 0` is undefined.

**Scaling note:** No cap on blueprints returned. With `errgroup.SetLimit(10)` and the expected 1:1 blueprint-to-deployment ratio, this handles ~20–50 blueprints comfortably within the 30s timeout. Accounts with 100+ deployments will hit 10+ sequential batches; if this becomes a real case, add a `limit` query param or switch to a pre-aggregated DB table.

---

## Client Changes

### New page: Activity

All components render server-provided data directly. No client-side aggregation.

| Component | Renders |
|-----------|---------|
| `TimeRangeSelector` | Segmented control: 7D / 14D / 30D / All. State in `useSearchParams`. |
| `StatCards` | 4 cards: cost, requests, tokens, active agents — `totals.*`, `daily_avg.*`, `change.*` (absent on "All") from summary. |
| `CostOverTimeChart` | `cost_over_time[]` — stacked bar, x = date, stacks = model. `recharts`. |
| `CostByModelChart` | `cost_by_model[]` — donut, pre-computed `cost_pct`. `recharts`. |
| `TopSpendersTable` | `blueprints[]` — sortable, all fields pre-computed. |

### New query hooks

**`apps/astro-client/src/api/queries/observability.ts`**
- `useAccountActivitySummary(account, from, to)`
- `useBlueprintsSummary(account, from, to)`

**`apps/astro-client/src/api/queries/keys.ts`**
- `observabilityKeys.activitySummary(account, from, to)`
- `observabilityKeys.blueprintsSummary(account, from, to)`

`staleTime: 5 minutes`, `refetchOnWindowFocus: false`.

---

## PR Breakdown

4 PRs. Server PRs (1 and 2) are independent of each other. PR 3 depends on both. PR 4 depends on PR 3.

---

**PR 1 — Server: patch account summary endpoint + DashboardStats migration**

Extend the existing `GetAccountLangfuseSummary` handler to be time-bounded and return all data the frontend needs pre-aggregated.

- Add `from`/`to` query params (replaces the existing `start_time`/`end_time` params on this handler; the rename is safe — no current client passes these params to the account summary endpoint, `DashboardStats.tsx` calls it with `{}`)
- Make two parallel `errgroup` calls: `GetDailyMetrics` (current), `GetDailyMetrics` (prior)
- Compute and return `cost_over_time[]` — shaped from `DailyMetric.Usage[]`, ready to render as stacked bar
- Compute and return `cost_by_model[]` — summed from same payload, with `cost_pct` pre-computed
- Add `totals.cost_usd`, `daily_avg.*`
- Add `change` object with % deltas vs prior period (cost, requests, tokens); omit key entirely when `from`/`to` are absent
- Add `active_agents` count from deployments table

**Breaking change:** The existing response shape (`total_traces`, `input_tokens`, `output_tokens` at root, `time_range` key) is replaced by the new `totals.*` / `period` shape. `DashboardStats.tsx` currently reads these root-level fields and will break when PR 1 lands. PR 1 must include a client-side migration — update `DashboardStats.tsx` (and `AccountObservabilitySummaryResponse` in `api.ts`) to use `totals.requests`, `totals.input_tokens + totals.output_tokens`, and `period` instead. No API versioning needed; the old shape has no external consumers outside this codebase.

---

**PR 2 — Server: blueprints-summary endpoint** *(Go only)*

New handler alongside existing ones in `observability_langfuse.go`.

- Fetch deployments for account, group by `agent_name`
- Fan out to Langfuse per deployment via `errgroup` with `errgroup.SetLimit(10)` — cap concurrent Langfuse calls to avoid rate limiting and connection pool exhaustion on accounts with many deployments
- Aggregate cost/tokens/requests per blueprint group; compute `cost_per_request`, `tok_per_request`, `p95_latency_ms` (max across deployments in the group), `top_model`
- Sort by `cost_usd` descending
- Register route in `main.go`

Reviewable alone. Pure Go. No client changes.

---

**PR 3 — Client: Activity page** *(depends on PR 1 + PR 2)*

New Activity page wired into the existing nav.

- `TimeRangeSelector` — segmented control, state in `useSearchParams`
- `StatCards` — 4 cards (cost, requests, tokens, active agents) with value, % change badge (hidden on "All"), daily avg
- `CostOverTimeChart` — stacked bar chart (`recharts`)
- `CostByModelChart` — donut chart (`recharts`)
- `TopSpendersTable` — sortable table, all columns from `blueprints[]`
- `useAccountActivitySummary` and `useBlueprintsSummary` hooks + query keys

Reviewable once PR 1 and PR 2 land. No Go changes.

---

**PR 4 — Polish + loading/empty states** *(depends on PR 3)*

- Skeleton loaders for each widget during fetch
- Empty state when account has no traces in the selected period
- Error states for Langfuse unavailability
- Responsive layout tweaks

Reviewable alone once PR 3 lands.

---

### Summary

| PR | Scope | Depends on |
|----|-------|-----------|
| 1 | Server: patch account summary + migrate DashboardStats.tsx to new shape | — |
| 2 | Server: blueprints-summary endpoint | — |
| 3 | Client: Activity page (all widgets + hooks) | PR 1, PR 2 |
| 4 | Client: polish, loading/empty states | PR 3 |

Critical path: PR 1 + PR 2 (parallel) → PR 3 → PR 4.

---

## Key Design Decisions

**1. All aggregation on the server.**
The client receives shaped, ready-to-render objects. No summing, grouping, or percentage math on the frontend. Business logic lives in one testable place.

**2. `cost_over_time` and `cost_by_model` from one Langfuse call.**
Both charts derive from `DailyMetric.Usage[]` — the server computes two different shaped arrays from the same payload. One Langfuse call, two chart-ready arrays.

**3. % change computed server-side.**
Two `GetDailyMetrics` calls (current + prior) run in parallel. The client receives a flat `change` object with pre-computed percentages.

**4. No new tables, no backfill.**
All reads from the existing `deployments` table and Langfuse API. No migration, no prod data risk.

**5. Blueprint-level P95 is max-of-P95s (accepted approximation).**
Per-blueprint P95 in the Top Spenders table uses `max` across deployments sharing an `agent_name`. This is not the true combined P95 (max-of-P95s can overstate latency if one deployment has extreme outliers), but it's acceptable because the expected norm is one deployment per blueprint per account. If multi-deployment blueprints become common, this can be revisited.

**6. No new packages.**
`recharts` already installed. No new env vars. All Langfuse client infrastructure exists.
