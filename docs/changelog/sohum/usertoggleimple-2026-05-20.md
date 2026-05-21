# feat(insights): per-user cost view + 3 related correctness fixes

## Summary

Org owners (CEO persona) wanted to answer "is one user driving most of our spend?" before signing off on a higher LLM budget. The agents-only Insights view aggregated through that question. This PR ships the new per-user view and three correctness fixes that surfaced while building it:

1. **Per-user view** — a `By Agent` / `By User` toggle that switches the table, chart, and filter bar to a user-grouped breakdown.
2. **Deleted-deployment scoping** — historical traces from undeployed agents no longer inflate account-level totals on either view.
3. **Unauthorized vs Unattributed split** — traces without a member-linked identity surface as two distinct, explainable buckets across the table, the filter chips, and the chart.
4. **Public-blueprint avatar resolution** — the new "Agents Used" column resolves cross-account avatars correctly (a public blueprint deployed under your org now renders the publisher's avatar, not a 404 placeholder).

## Design

### 1. Per-user view

**Server.** `GET /accounts/:account/observability/summary` accepts `?group_by=user`. When set, the response additionally populates `cost_over_time_by_user` (per-date user breakdown for the stacked chart) and `cost_by_model` is blanked since the donut isn't rendered in user view. New endpoint `GET /accounts/:account/observability/users-summary` returns per-user `requests/cost/tokens/last_seen/agents_used`. Both live in `apps/astro-server/handlers/observability_langfuse.go`.

Two parallel Langfuse queries back the users endpoint (`g, gCtx := errgroup.WithContext(...)`):

- `Q_main`: `view: "traces"`, dimensions `[userId]` time-bucketed daily — produces totals per user + `last_seen`.
- `Q_tags`: `view: "traces"`, dimensions `[userId, tags]` — produces the deployment fan-out per user, mapped to `agent_name` via the deployments table.

`buildUsersSummary` merges them into one row per user; `agents_used` is deduped by `account/name` (so two publishers of the same agent name don't collapse) and capped at `maxAgentsPerUser = 10`.

> **Why `view: "traces"` and not `observations`?** Mirrors the legacy POC that used `observations` and double-counted spans within a trace, producing chart-vs-table totals that didn't reconcile. Switched to `traces` in commit `e2d55c1` and aligned the chart query in commit `cd0ba125`.

**Tag-shape defense.** Langfuse can return the `tags` group-by value as either a single string or an array. The first cut asserted string and silently dropped every array-shaped row → empty `agents_used` in preview. Fixed by the `tagStrings(v any) []string` helper. Regression test: `TestBuildUsersSummary_TagsReturnedAsArray`.

**All-time timestamp backfill.** Langfuse's `/api/public/metrics` endpoint (used by `cost_over_time_by_user`, `Q_main`, and `Q_tags`) 400s when `fromTimestamp` / `toTimestamp` are empty strings — unlike `/api/public/metrics/daily` which accepts the empty form. With `?range=all`, the client passed empty timestamps through and every `GetMetrics` call failed; the errgroup bubbled the error and the handler returned 502 on hard refresh. The `metricsTimeRange(from, to)` helper substitutes a 5-year lookback ending now whenever both inputs are empty, applied at all three `GetMetrics` call sites. Long enough to be "all-time" in practice and keeps both Langfuse endpoints' semantics aligned.

**Client.**

- `apps/astro-client/src/components/activity/ViewToggle.tsx` + `parseActivityView`: `?view=users` toggles the view; `?view=` (or anything unrecognized) falls back to agents.
- `apps/astro-client/src/components/activity/use-insights-data.ts`: `useUsersInsightsData` mirrors `useInsightsData` — reads `useAccountActivitySummary(…, groupBy: "user")` + `useUsersSummary` + `useAccountMembers`. Loading state factors in the members query so classification can be trusted before the bucket renders.
- `apps/astro-client/src/components/activity/TopSpendersTable.tsx`: discriminated union on `mode: "agents" | "users"`. Members render as sortable rows; the bucket rows pin to the bottom. `<MetricsCells row={…} />` is the shared row body.
- `apps/astro-client/src/pages/Insights.tsx`: `AgentsTab` / `UsersTab` each call their hook once; loader-driven cache priming via `usePrimeQueryCache` matches the `orgswitchingfix-2026-05-19.md` pattern (no more `initialData`). For the users view, the loader also fetches the account members list and primes it under `accountKeys.members(account)` — without this, the table flashed a ghost-row skeleton on hard refresh while `useAccountMembers` fetched, since classification depends on it (agents view has no equivalent gating dependency). `shouldRevalidate` re-runs the loader only on `view` param changes — range/agents/users URL changes stay CSR.

### 2. Deleted-deployment scoping

Previously the account aggregator called `GetDailyMetrics(account)` once. After a deployment was deleted (`status = 'undeployed'`), Langfuse still held its traces and they kept inflating account totals — visible as deleted-agent dollars on the Insights page.

The new pipeline (`mergedDailyMetrics` in `observability_langfuse.go`):

```
visibleDepIDs := GetVisibleDeploymentsByAccount(account)   // status != 'undeployed'
mergedDailyMetrics(ctx, client, log, visibleDepIDs, from, to)
```

fans out per-deployment `GetDailyMetrics` (bounded to 10 concurrent) and merges per-date. It also returns `activeDeps` — the set of deployment IDs with ≥1 trace in the period — used to derive `active_agents` from trace presence, not from the deployment-table snapshot. Partial fetch failures log a warning; all-fail returns an error so the caller responds 500.

> **Why fan out?** Langfuse's `/api/public/metrics/daily` accepts a single `tags` query param — no equivalent of the newer `/api/public/metrics` `arrayOptions any of` filter. Switching endpoints would let us scope in one call but the daily endpoint returns rich per-model usage (`DailyMetricUsage`) that `buildAccountSummary` feeds into `cost_by_model`; reconstructing that from `/metrics` needs a second `model`-grouped query and equivalent merge logic. Documented as the migration path for when very-large-deployment-count accounts make the fan-out latency-visible.

> **Applies to both views.** This affects the legacy agents view too — by design. Deleted-deployment traces no longer surface anywhere. Mirrors the deployment-detail page contract.

Server tests: `TestMergedDailyMetrics_MergesPerDateAndTracksActiveDeps`, `_AllFailReturnsError`, `_PartialFailReturnsNoError`.

### 3. Unauthorized vs Unattributed split

A single `classifyUserId(uid, memberIds)` helper in `apps/astro-client/src/components/activity/user-classification.ts` (the domain module) buckets every trace's `user_id` into one of:

- **Member id** — resolves to the WorkOS member's display name + avatar.
- **`UNAUTHORIZED_USER_KEY`** — `user_id` is set but isn't a member. Typically a trace from an enabled adapter (Slack, Discord, etc.) where the user hasn't authorized that adapter to link their identity. They may be a member of the org; their adapter identity just isn't linked.
- **`UNATTRIBUTED_USER_KEY`** — no `user_id` at all. Background jobs, SDK calls, crons.

The classifier is consumed by `useUsersInsightsData` once and flows into the filter chips (`UserFilterBar.tsx`), the chart series (`buildUserCostOverTime`), and the top-spenders rows (`TopSpendersTable.tsx`). Both bucket rows pin to the bottom of the table with their own tooltips; the unauthorized row carries a `· N users` suffix.

`normalizeUserID(s)` (server-side) maps the SDK-emitted `"-"` sentinel to `""` at every Langfuse read site so the frontend only handles one shape of "no user."

> **Why two buckets and not one?** The CEO persona wanted to tell apart "Slack traffic" (unauthorized — a real human, just not linked) from "background jobs" (unattributed — no user at all). Lumping them together hides whether to invest in adapter identity-linking vs. SDK instrumentation. See `feedback_loading_ux_strategy.md` history if curious why we didn't surface "Unknown user" as the placeholder.

### 4. Public-blueprint avatar resolution

The "Agents Used" column renders small `<BlueprintIdentity>` avatars per agent. The avatar URL is `${cdn}/avatars/agents/${account}/${name}.jpg` — the `account` slug has to be the **publishing** account, not the deploying org, for public-blueprint deploys (where `deployments.source_account_id` is set and differs from `account_id`). A first cut passed the active account through to every chip → 404 placeholder for any cross-account / public deployment.

Fix is server-side in `observability_langfuse.go`:

- `depToAgent` is now `map[string]UserAgentRef` where `UserAgentRef { Name, Account }` carries the publishing account.
- `GetAccountUsersSummary` collects distinct `SourceAccountID`s across visible deployments, batches `accountStore.GetByID(...)` to look up account names, and stamps each entry with either the source account (`source_account_id` set and different from the deploying account) or the deploying account.
- API surface: `UserSummaryEntry.agents_used` is now `[]UserAgentRef`, not `[]string`.

Client-side: `AgentsUsedChips` dropped its single `account` prop; each entry carries its own. Tooltip + deep-link route both use the entry's account. Lives in `apps/astro-client/src/components/activity/AgentsUsedChips.tsx`. Test that exercises the cross-account path: `AgentsUsedChips.test.tsx` "links each avatar under its own publishing account".

> **Where to look next time this breaks.** If a customer reports placeholder avatars in the column, the chain to walk is: (1) Langfuse `Q_tags` returning the right `deployment:<id>` tags → check `tagStrings()` parsing; (2) `GetVisibleDeploymentsByAccount` returning the deployment → check `status != 'undeployed'`; (3) `SourceAccountID → account.Name` resolution → confirm `accountStore.GetByID()` succeeded (silent fallback logs `Debug`); (4) the CDN actually has an avatar at the published path → upload via the avatar flow on the publisher's account.

## Migration

None required. New route params (`?view=users`, `?group_by=user`) and the new `/users-summary` endpoint are additive. The `agents_used` schema change (string → object) only affects the new users-summary endpoint introduced by this PR — no existing callers depend on the old string shape. Deleted-deployment scoping is a silent correction; accounts that have deleted deployments will see their account totals drop to the true (lower) number on the next request.
