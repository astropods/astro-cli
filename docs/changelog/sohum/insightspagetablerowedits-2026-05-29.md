# Insights: per-deployment rows (phase 1)

## Summary

The Insights agents-view table used to render one row per `agent_name`,
rolling up multi-region deployments into a single entry. Saswat's
framing: in this product an **agent == a deployment**, not a blueprint.
This PR decouples them — every deployment now gets its own row, and
clicking a row deep-links straight to that deployment's Monitor tab.

(Tombstone treatment for deleted deployments is the phase-2 follow-up;
this PR is the data-shape + routing change underneath it.)

## Design

### Backend — replace `blueprints-summary` with `deployments-summary`

Renamed `GET /api/v1/accounts/:account/observability/blueprints-summary`
→ `…/observability/deployments-summary`. Old endpoint had no other
consumers (verified — astro-queen, astro-cli, e2e tests, packages,
scripts all clean), so the route swap is total, not additive.

The Langfuse fan-out flow is unchanged — the same per-deployment
`fetchDeploymentDaily`, batched P95 query, and batched Q_tags inversion
runs as before. What changed is the final aggregation step:

- `buildBlueprintsSummary(metrics, tagsRows, depToAgent)` →
  `buildDeploymentSummary(metrics, tagsRows, deployments)`
- No more `groups := map[agent_name]*group` reduction. Each
  `deploymentMetrics` entry becomes one `DeploymentSummaryEntry`.
- `users_used` inversion keys by `deployment_id` instead of
  `agent_name`. Same user on two regional deployments shows up under
  both entries (no cross-deployment dedupe).
- `deploymentMetrics` now carries `DeploymentID` so the build helper
  doesn't need positional pairing against the deployments slice.

New response shape (`DeploymentSummaryEntry`) carries the same totals
the old blueprint entry did, plus `deployment_id`, `display_name`, and
`namespace` so the frontend can label rows.

Existing thin type `DeploymentSummaryEntry` (the
`{total_traces, last_trace_at}` bulk projection) renamed to
`DeploymentTraceSummary` to free up the name — only one consumer needed
updating.

### Frontend

- `lib/api.ts`: types renamed
  (`AccountBlueprintsSummaryResponse` → `AccountDeploymentsSummaryResponse`,
  field `blueprints` → `deployments`); method renamed
  (`getAccountBlueprintsSummary` → `getAccountDeploymentsSummary`).
- `api/queries/observability.ts` + `api/queries/keys.ts`:
  `useBlueprintsSummary` → `useDeploymentsSummary`; cache key string
  `'blueprints-summary'` → `'deployments-summary'`.
- `use-insights-data.ts`: chart series key is now `deployment_id`
  (multi-region deployments get distinct lines on the Agent-spend
  chart); each row gets a `seriesLabels` entry mapping id → human
  label for the legend / tooltip. Dropped the dead `selectedAgents`
  filter param + `ALL_AGENTS_KEY` constant that were holdovers from
  the multi-select chip filter retired in the previous PR.
- `TopSpendersTable` agents mode: takes `deployments` instead of
  `blueprints`. Each row links directly to
  `/{account}/agents/{deployment_id}/monitor` — **no picker** since
  the agent-name → deployment relationship is 1-to-1 now.
- `Insights.tsx` loader: primes the new `deploymentsSummary` cache
  entry. Search haystack matches against
  `agent_name + display_name + namespace`.

### Clickable legends

While in the chart code, both legends are now clickable to toggle
series visibility:

- **Agent spend over time**: click any deployment name in the legend
  to hide that line / bar. Helps when one outlier deployment is
  dominating the y-axis scale.
- **People spend over time**: click `By People` ↔ `Total spend` to
  isolate either axis on its own.

Hidden items get strikethrough + 50% opacity. Last visible series is
locked on so users can't accidentally empty the chart. Uses
`aria-pressed` for screen-reader state.

### Misc table polish (squashed in)

- Tooltips: `text-balance` from the Tooltip primitive was creating
  orphan widow lines on multi-line content (e.g. "available." on its
  own line under the not-instrumented warning). Override with
  `[text-wrap:initial]` on the multi-line tooltips so text fills each
  line up to `max-w`.
- Agents-Used chip hover tooltips: now render for every chip
  regardless of deployment count. Wrap order is
  `<Tooltip><span><AgentNameLink/></span></Tooltip>` so radix's
  Tooltip and DropdownMenu refs sit on different DOM nodes.
- Agent-name column dropped the `namespace` mono suffix next to the
  display name — too noisy when most users have one deployment per
  agent.

## Migration

No external migration. `blueprints-summary` had no out-of-tree
consumers; the rename is a complete swap.

## Files touched

### Backend

- `apps/astro-server/handlers/observability_langfuse.go` — handler +
  build helper renamed, no rollup
- `apps/astro-server/handlers/responses.go` — types renamed; existing
  `DeploymentSummaryEntry` thin type → `DeploymentTraceSummary`
- `apps/astro-server/main.go` — route registration
- `apps/astro-server/handlers/observability_deployments_test.go` —
  renamed from `…_blueprints_test.go`; assertions cover the no-rollup
  invariant (two deployments of one agent_name → two entries) and
  per-deployment `users_used` scoping

### Frontend

- `apps/astro-client/src/lib/api.ts` — types + client method
- `apps/astro-client/src/api/queries/observability.ts` — hook
- `apps/astro-client/src/api/queries/keys.ts` — cache key
- `apps/astro-client/src/components/activity/use-insights-data.ts` —
  slice helpers + summary builder + chart series key + `seriesLabels`
- `apps/astro-client/src/components/activity/TopSpendersTable.tsx` —
  per-deployment rows, monitor-tab links
- `apps/astro-client/src/components/activity/CostOverTimeChart.tsx` —
  clickable legend
- `apps/astro-client/src/components/activity/ActiveUsersSpendChart.tsx` —
  clickable legend
- `apps/astro-client/src/components/activity/AgentsUsedChips.tsx` —
  tooltip always renders
- `apps/astro-client/src/pages/Insights.tsx` — wires new hook +
  loader prime; search haystack
- Test fixtures + assertions updated in
  `build-filtered-summary.test.ts` and `TopSpendersTable.test.tsx`
