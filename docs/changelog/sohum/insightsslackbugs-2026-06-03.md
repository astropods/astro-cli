# Insights / Slack: two follow-up fixes from preview testing

## Summary

Two issues surfaced while validating the Slack user-attribution work in preview:

1. **Slack deep link broke for previously-linked-then-disconnected users.** Users who linked via oauth and then disconnected lost their Insights deep link permanently — the chip rendered as a plain non-clickable span. New users who never linked, and currently-linked users, were unaffected.
2. **People-tab "Agents Used" column collapsed deployments by blueprint.** Two deployments of the same blueprint surfaced as one chip in the People table even though the Agents tab shows them as separate rows.

## Design

### Deep link fix

The bug is an interaction between two correct-in-isolation pieces:

1. **Schema**: `slack_identity_mappings_unique UNIQUE (team_id, slack_user_id)` allows at most one row per (team, user) pair regardless of source or revoked status.
2. **Security guard on live-ingest** (`UpsertObserved`): the `ON CONFLICT DO UPDATE … WHERE source = 'observed'` clause prevents reviving a revoked oauth row, since revocation is a deliberate user action.

Combined, a previously-linked-then-disconnected user is stuck: the revoked oauth row occupies the unique key, blocking live-ingest from inserting a fresh observed row. The directory join's `WHERE revoked_at IS NULL` filter then excludes the (revoked) row, so `users-summary` returns no `slack_team_id` for the user — no deep link.

`DirectoryEntriesForSlackUsers` now returns rows regardless of `revoked_at` status, but masks `workos_user_id` to empty for revoked rows. The two consumers of `DirectoryEntry` handle this cleanly:

- **Deep link path**: `team_id` is unchanged whether the row is revoked or not.
- **Metrics merge path** (`mergeLinkedSlackRows`): only folds when `WorkOSUserID` is non-empty. Revoked rows have an empty `WorkOSUserID`, so post-disconnect spend stays attributed to the Slack chip — never silently re-folded into the WorkOS account the user explicitly unlinked from.

`DISTINCT ON` now prefers non-revoked rows via `ORDER BY slack_user_id, (revoked_at IS NULL) DESC, created_at DESC`, so an active row wins over a revoked one when both exist (cross-workspace edge case).

### Per-deployment chips

`UserAgentRef` grows a `deployment_id` field. Server-side dedup in `buildUsersSummary` switches from `account/name` to `deployment_id`, so two deployments of the same blueprint each surface as their own ref. The client mirrors this in `aggregateUsers` (the per-period merge that builds bounded-window user rows).

`AgentsUsedChips` rewires:

- Keyed by `deployment_id` (one chip per deployment, not per blueprint).
- Each chip links directly to `/{account}/agents/{deployment_id}/monitor` — no more `AgentNameLink` picker, since the deployment is already specific.
- Tooltip enriches with `display_name (namespace)` from the existing `deploymentsByAgent` index, so two identical-avatar chips are distinguishable on hover.
- Falls back to `/{account}/{name}` (blueprint detail) when the deployment isn't in the live-only `deploymentsByAgent` index — covers archived-only spend and cross-account public deploys.

The chip avatar is still the blueprint avatar (no per-deployment avatar today). Two distinguishers per the user's design choice: tooltip text + click target.

## Migration

None. The deep-link fix is SQL-only inside one query — the security guarantee on `UpsertObserved` is unchanged, revoked oauth rows still don't get auto-revived on the next message. The chips change adds a JSON field on the existing response and reshapes one component's rendering; no API contract break for older callers (the field is additive).
