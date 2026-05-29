# Insights: deleted-agent persistence + bug fix for `undeployed_at`

## Summary

Insights used to drop deleted deployments outright — their spend
disappeared the moment an agent was torn down, even though Langfuse
still had the traces. This PR persists that spend behind a new
**Show deleted** toggle on the Agents view: opt in to surface
tombstoned deployments in the table, charts, and headline KPIs,
scoped to the selected date range. Live agents get a small green
status dot trailing the name; deleted agents get a red dot whose
hover reveals the deletion date.

A latent bug in the undeploy worker — `undeployed_at` was never being
stamped — is fixed in the same change, with a one-shot backfill that
restores the historical column from `status_changed_at`.

## Design

### Backend — opt-in tombstone discovery (`deployments-summary`)

A new query param `?include_archived=true` controls whether the
`deployments-summary` endpoint surfaces archived deployments. Default
off — the page's hot path stays as fast as before for the common case.

When the flag is on, the existing batched Q_tags Langfuse probe
(originally feeding `users_used`) doubles as the discovery channel.
Dropping its tag filter returns every `deployment:*` tag that had
traces in the window; subtracting the live deployment set leaves the
archived-with-spend universe. Those IDs (sorted deterministically and
capped at `maxTombstones = 50`) are loaded from the deployment store
via a new `GetDeploymentsByIDsForAccount`, then fanned out for P95 +
daily metrics in a second errgroup. Accounts with no tombstones pay
no extra cost — the discovery set is empty, the second phase skipped.

Langfuse's `arrayOptions` filter is set-membership only, so there's
no prefix operator to bound the unfiltered query to `deployment:*`
tags. A `log.Debug` records the response row count on this path so
runaway responses are observable.

`DeploymentSummaryEntry` carries two new fields:

- `is_archived` — boolean, true for any entry sourced from the
  discovery pass. Source of truth for the tombstone styling.
- `undeployed_at` — optional RFC3339 timestamp, used only for the
  date in the hover tooltip. Independent of `is_archived` because
  some archived states (e.g. `status='undeploying'` mid-tear-down)
  have a null timestamp.

### Backend — chart + headline KPIs (`/observability/summary`)

The account-summary endpoint is also gated on the same
`include_archived` flag. When on, `accountDailyMetrics` and the
user-grouped Q_tags query both drop their visible-deployment tag
filter — archived deployments' historical traces roll into the
People-spend-over-time chart and the headline KPIs without a second
discovery pass (a simple roll-up doesn't need to know which
deployments are archived).

### Bugfix — `updateStatusTx` now stamps `undeployed_at`

`updateStatusTx` was the canonical entry point for status changes
but didn't touch `undeployed_at`. The undeploy worker compensated by
calling `MarkUndeployedByID(...)` after `UpdateStatus(StatusUndeployed)`,
but the helper's SQL had a `WHERE status = 'active'` guard — by the
time it ran, status was already `'undeployed'`, so the UPDATE matched
zero rows silently. Every soft-delete since this code shape existed
had `undeployed_at = NULL`.

The fix: `updateStatusTx` now stamps `undeployed_at` in the same
UPDATE when transitioning to `'undeployed'`, via a CASE guard that
preserves the original timestamp on subsequent transitions.
`MarkUndeployedByID` is deleted.

The `$2` placeholder is explicitly cast to `text` in the SQL —
without the cast, pq's parameter-type deduction throws "inconsistent
types deduced for parameter $2" because the same placeholder appears
both as a varchar column value and as a text literal comparison.
This was caught by a new integration test that exercises both the
stamp-on-first-transition path and the idempotency of the CASE guard.

### Frontend — UI + URL state

A `?archived=true` URL parameter persists the toggle.
`useInsightsData` / `useDeploymentsSummary` / `useActiveSpendSeries`
all accept an `includeArchived` opt that flows into their query keys
and `include_archived=true` query params. The loader prime reads the
URL flag too, so deep-links to `?archived=true` land warm on both
endpoints.

The toggle (a `<Switch>` + "Show deleted" label) sits inline with the
People/Agents view toggle, agents-mode only.

Table treatment for deleted rows:

- Text muted via `text-muted-foreground`; avatar at 60% opacity
- Live rows deep-link to the deployment's Monitor tab
  (`/{account}/agents/{deployment_id}/monitor`); deleted rows render
  a non-interactive span so a click doesn't 404 on a torn-down
  deployment
- Hovering the identity unit (avatar + name) shows
  "Deleted MMM DD, YYYY", or just "Deleted" when the timestamp is
  unknown

People view's Agents Used chip column gets the same treatment: when
a chip's agent_name has no live deployment (i.e. exists only as
archived), the avatar mutes to 60% opacity and the hover tooltip
suffixes "(deleted)". Detection is the absence of a live deployment
in `deploymentsByAgent`, so it's free — no new data fetched.

### Frontend — view-toggle smoothness

Two motion treatments to keep the page feeling stable as data shifts:

- `AnimatePresence mode="wait"` around the whole table block, keyed
  by the active view. Toggling People ↔ Agents crossfades the table
  contents (180ms easeOut) instead of snapping.
- `motion.create(TableRow)` provides `MotionTableRow`, which wraps
  the existing `TableRow` chrome (border, `data-slot`, interactive
  hover state) and adds opacity transitions inside an
  `AnimatePresence`. Deleted-row insertion on toggling Show deleted
  fades in/out rather than snapping.

### Backfill — `undeployed_at` from `status_changed_at`

`status_changed_at` carries the exact moment of the undeploy
transition for every legacy soft-delete, so it's the natural
backfill source. Idempotent — after the first run the WHERE clause
matches zero rows, since `updateStatusTx` stamps the column going
forward.

```sql
UPDATE deployments
SET undeployed_at = status_changed_at
WHERE status = 'undeployed' AND undeployed_at IS NULL;
```

Local dev gets it automatically via `apps/astro-server/scripts/dev.sh`,
which runs the UPDATE on every boot. Kept out of CI on purpose —
one-shot data fixes shouldn't live in the recurring migrate
workflow.

## Migration

- After deploy, run the SQL above once against the prod DB. One-shot;
  the SQL is in `docs/changelog/sohum/deleteddeployments-2026-05-29.md`
  for copy-paste.
- Frontend consumers passing `undefined` for `from` / `to` to
  `useDeploymentsSummary` are unaffected — the new `includeArchived`
  option is optional and defaults to false.
