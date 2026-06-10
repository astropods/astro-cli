## Summary

Insights needs to identify Slack-only agent users without forcing them to create Astro accounts first. This change turns the Slack directory used by Insights into a workspace-scoped directory, so unlinked Slack rows can show Slack profile metadata and stable deep links when the observed directory has one known workspace for that Slack user.

## Design

The existing Slack authorization path is the source of Slack runtime identity. For every allowed Slack request, Astro Server receives the raw Slack user ID plus the Slack team ID and stores that live-ingest record in `slack_observed_users` keyed by `(team_id, slack_user_id)`, which is enough to create deterministic Slack deep links.

Astro Server owns Slack profile enrichment. The existing Astro account Slack OAuth flow already requests the `users:read` user scope, so when an Astro user connects or reconnects a Slack workspace, the callback calls `users.list` once with that user token and bulk-upserts the workspace directory into `slack_observed_users`. This stores Slack display name, username, avatar, bot/deleted flags, and workspace scope before those users need to interact with an agent.

Deployed agents do not need to call Slack profile APIs and do not need to send Slack profile metadata through authorization query params. This avoids requiring every deployed agent bot token to gain a new scope and keeps PII out of per-message authorization URLs.

Insights treats `(team_id, slack_user_id)` as display metadata for unlinked Slack rows only when the observed directory can provide a deterministic team ID. Langfuse rows still carry the bare Slack user ID; Astro Server enriches that row only when the Slack user has exactly one known workspace. If multiple workspaces are possible, Astro Server leaves the row raw rather than picking a workspace or duplicating spend.

Linked Slack identities still collapse into the corresponding Astro member row through `slack_identity_mappings`. Observed-only identities stay as Slack rows and expose display name, username, avatar, team ID, and workspace metadata to the client. The People table uses the new identity key for row identity, range slicing, filters, and list keys so duplicate Slack user IDs from different workspaces do not overwrite or collapse each other.

The Agents table now uses the same Slack directory path for the Used by column. The deployment summary response keeps the existing `users_used` array for compatibility and adds `users_used_details` as the display-ready identity list. The client prefers the richer list when present, falls back to raw IDs for older responses, and renders Slack names, avatars, and deep links through the same helpers used by the People table. Workspace metadata stays on the API response for deterministic identity keys, search, and future invite flows.

When Insights detects Slack rows that are still missing deterministic workspace/profile details, it shows a Slack details action in the table header. The action reuses the existing Slack OAuth connect flow, returns to Insights, and refetches the affected Insights caches after the callback has refreshed `slack_observed_users`.

The Slack app setup uses the Astro account Slack OAuth app's `users:read` user scope. Email is intentionally out of scope; the enrichment only needs Slack profile names and avatars.

## Migration

Apply the updated database schema so `slack_observed_users` has the Slack profile columns. No deployed agent bot scope change is required. Directory sync runs when a user connects or reconnects their Astro account to a Slack workspace; existing linked workspaces can be covered by a separate backfill if needed.
