# Warn when a Slack grant targets a user who hasn't connected Slack

## Summary

The grants editor lets an admin add a specific user to a Slack-typed grant, but those grants are evaluated at request time against the `slack_identity_mappings` table — a per-`(team_id, slack_user_id)` link the user has to create themselves via the "Connect Slack" settings flow. Until that link exists the grant resolves to nothing and the user can't invoke the agent. Today the UI gives no signal, so admins find out by trial.

## Design

Surface a warning in the grants editor whenever a per-user Slack grant targets a member with zero linked Slack workspaces. The check is workspace-agnostic on purpose: a deployment isn't pinned to a specific Slack workspace ahead of time, so the only fact we can assert with no false positives is "this member has linked nothing — definitely can't invoke". A member with at least one linked workspace is shown without warning, leaving the workspace-vs-routing match as a judgment call for the admin.

### Surfacing the data

`GET /api/v1/accounts/:account/members` already batch-fetches WorkOS roles and profiles; we extend it rather than introduce a parallel endpoint:

- `MemberResponse` gains `slack_workspaces: SlackWorkspaceRef[]` (team id, name, domain, icon).
- A new `slackidentity.Store.ListByWorkOSUsers([]string)` does the lookup in one query using `workos_user_id = ANY($1)`, so the cost is one indexed scan regardless of member count.
- The slack lookup is best-effort — a query failure logs and falls through with empty workspaces rather than blocking the listing.

One round trip, one query key, no new invalidation story.

### UI

`GrantsEditor` threads `adapter` into `GrantRow` and `MemberPicker`. When `adapter === "slack"` and the resolved member has `slack_workspaces.length === 0`, both surfaces render an `AlertTriangle` icon (themed `text-destructive`) next to the display name with a tooltip explaining the consequence. The icon sits inline with the name so it's visible without scanning across the row.

The picker stays neutral beyond the icon: pre-adding a teammate who hasn't linked yet is a legitimate flow, so the row is still selectable and uncoloured.

## Migration

No migration required. No schema, API path, or grant-evaluation changes — only an additive field on the member listing response and a new UI affordance.
