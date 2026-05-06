# Per-user grants on Slack

## Summary

Per-user authorization grants previously only worked on web. Slack identity arriving at the messaging container (`slack_user_id`) is opaque from the server's perspective — same user_id can refer to different humans across workspaces, and there was no way to map it to a WorkOS user. Result: any `user_id` grant on a slack-deployed agent silently never matched, and the validator + schema CHECK rejected such grants outright at write time.

This change adds the missing link: a slack_user can connect their Slack account from the settings panel, and from then on any per-user grant on slack matches their identity the same way it does on web. Multi-workspace by design — one user can link as many workspaces as they have.

## Design

**Identity mapping is the new primitive.** A new `slack_identity_mappings` table keys `(team_id, slack_user_id) → workos_user_id` plus display fields (`team_name`, `team_domain`, `team_icon_url`, `slack_username`). The pair `(team_id, slack_user_id)` is the unique key — slack `user_id`s are only unique within their team, so two workspaces with overlapping ids are independent rows. Soft-deleted via `revoked_at` so disconnect/re-link history is auditable.

**Three-layer resolution.** The end-to-end matching flow:

1. **Messaging container** (`modules/messaging`, [PR #29](https://github.com/astropods/messaging/pull/29)) — every slack ingress (events, slash commands, app mentions, reactions, button clicks) threads the workspace `team_id` through to `Authorizer.Allowed(_, _, _, _, identityScope)`. The HTTP client sends it to the server as `identity_scope=Txxx`. Cache key includes scope so two workspaces with overlapping ids never share a slot.
2. **Server resolver** (`handlers/authorization.go`) — when slack identity arrives with a scope, calls `slackidentity.Store.Lookup(scope, identityID)`. On hit: emits a User candidate (the resolved WorkOS user_id) plus the user's account candidates, so per-user grants match via the same path as web. Always also emits the deployment's owning-account candidate, so `org` and `anyone` grants keep matching for unmapped users — additive, no regression.
3. **Storage** — `slack_identity_mappings` populated by the link flow (below). Resolution at request time is one indexed `SELECT` keyed on `(team_id, slack_user_id) WHERE revoked_at IS NULL`.

The `validateAuthorizationSpec` slack→`user_id` rejection and the `deployment_authorization_grants_user_web_only_check` schema CHECK are dropped, since user grants on slack are now meaningful.

**Link flow uses raw Slack OAuth, not WorkOS Pipes.** The natural fit for Pipes was undermined by Slack's token model: Pipes' `GetAccessToken` returns a bot token (`xoxb-…`), and `auth.test` on a bot token resolves to the bot user, not the human installer. Verified live — slack identity is unrecoverable through Pipes for our use case. The link flow runs slack OAuth directly:

```
POST /api/v1/accounts/:account/slack/connect
   ↓ (sets HttpOnly Secure SameSite=Lax CSRF state cookie)
   ↓
slack.com/oauth/v2/authorize?user_scope=users:read,team:read&...
   ↓ (user authorizes)
   ↓
GET /api/v1/accounts/:account/slack/callback?code=...&state=...
   ↓ (state cookie verified via constant-time compare)
   ↓ (POST oauth.v2.access → authed_user.id is the human's slack_user_id)
   ↓ (best-effort team.info → team display fields + icon URL)
   ↓ (best-effort users.info → slack_username)
   ↓
slack_identity_mappings.Upsert(...)
```

`user_scope` (not `scope`) is the load-bearing detail — produces a user token whose authed_user.id is the linker, not a bot. Slack identity comes straight from `oauth.v2.access`'s `authed_user.id`; `team.info` and `users.info` are best-effort enrichment for the settings UI (workspace icon, friendly handle) and never block the link.

**Multi-workspace is data-layer-native, not flow-layer-native.** WorkOS Pipes scoped connections by `(provider, user, org)` — exactly one — which would have blocked stacking workspaces. With raw OAuth there's no such constraint: each link drops a fresh row in `slack_identity_mappings` keyed on `(team_id, slack_user_id)`. The settings panel renders one row per linked workspace with per-row disconnect; "Add workspace" is always visible.

**Settings UI** lives in `/settings/account` next to the GitHub panel, mirrors its conventions (status query + connect mutation + per-row disconnect). Each linked workspace row shows the real workspace icon (from `team.info`), the team name, and the linked handle. Falls back to a generic Slack svg when the workspace uses slack's default icon.

**CSRF on the OAuth state.** 32-byte random nonce written to an `HttpOnly` `Secure` `SameSite=Lax` cookie on `/connect`, verified on `/callback` via `subtle.ConstantTimeCompare`. Cleared on every callback exit path so a stale cookie can't be reused.

## Migration

**No existing data to migrate.** The `slack_identity_mappings` table is new; no per-user grants on slack existed before this change because the validator would have rejected them.

**Required env vars on astro-server:**

| Var | Purpose |
|---|---|
| `SLACK_CLIENT_ID` | Slack app credentials from api.slack.com/apps → Basic Information |
| `SLACK_CLIENT_SECRET` | Pair of `SLACK_CLIENT_ID`. Treat as a secret. |
| `SLACK_CALLBACK_URL` | Public base URL the slack OAuth `redirect_uri` is built from. Optional — falls back to `FRONTEND_URL`. Distinct from `GITHUB_CALLBACK_URL` so each integration's dev tunnel can differ. |

**Slack App configuration** (one-time, at `api.slack.com/apps`):

- **OAuth & Permissions → User Token Scopes**: `users:read`, `team:read`. (No bot scopes — we don't install the app, just identify the user.)
- **OAuth & Permissions → Redirect URLs**: add `${SLACK_CALLBACK_URL}` as a prefix (Slack matches by prefix; the path-with-query is constructed at runtime).

**Deployment ordering with messaging:** the resolver is backward-compatible — slack requests without `identity_scope` skip the mapping lookup and behave exactly like before. The messaging client's `Allowed()` signature gained the scope param ([PR #29](https://github.com/astropods/messaging/pull/29)); older messaging containers continue to work against the new server. New messaging containers send `identity_scope` to old servers and the server simply ignores the extra query param. So the two repos can ship in either order without coordination.
