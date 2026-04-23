# GitHub Account Disconnect & Connection State Propagation

## Summary

Several issues with the GitHub OAuth integration that affected connection state reliability: the account-level disconnect toggle would flip back on after clicking, the WorkOS Pipes token was never actually revoked on disconnect, OAuth redirects after connecting landed on a 404, and the BP panel didn't show the GitHub username when a user was OAuth'd but hadn't linked a repo yet.

## Design

**Server: WorkOS Pipes token revocation**

Added `DeleteConnection` to `internal/pipes/client.go` — calls `DELETE /user_management/users/:user_id/connected_accounts/:provider`. This is a best-effort call made at the end of `GitHubAccountDisconnect`, after all `github_connections` rows and webhooks are removed. On failure it logs a warning and continues, so a WorkOS outage doesn't block the user from disconnecting.

**Client: optimistic disconnect that doesn't race**

`useGitHubAccountDisconnect.onSuccess` now:
1. Broadly invalidates `['github', account]` so BP panels see the disconnect
2. Calls `cancelQueries` on `accountStatus` to stop any in-flight refetch from overwriting
3. Sets `accountStatus` cache directly to `{ connected: false }`

The account settings `handleDisconnect` also does a synchronous `setQueryData({ connected: false })` before the mutation fires, so the UI flips immediately. On error it rolls back to the previous value.

**Client: connected state propagates from BP link**

`useGitHubLink.onSuccess` now also invalidates `githubKeys.accountStatus(account)` so that after a user links a repo via the BP panel, the account settings page reflects the connected state without a reload.

**Client: BP panel header username**

The `githubLogin` value shown in the panel header falls back to `accountStatus?.github_login` when the agent has no linked repo yet. This covers the state between completing OAuth and picking a repo.

**Client: OAuth redirect path fix**

The `redirectTo` sent to the server was `/${account}/settings/account` (wrong — settings routes have no account prefix) and included `?github_connected=true` (causing a double-`?` when the server appended the same param). Fixed to `/settings/account`.

**Mock backend & e2e tests**

Added missing endpoints to the mock backend:
- `GET /api/v1/accounts/:account/github` — returns `{ connected, github_login }`
- `DELETE /api/v1/accounts/:account/github` — clears `githubAccountConnected` and all connections
- `POST .../github/connect` now returns `github_login` so the repo dialog opens without a redirect

Three new e2e tests in `github-connection-flow.spec.ts` covering the diagram cases:
1. Linking a repo via BP panel → account settings shows globally connected
2. Unlinking a BP repo → account-level connection survives (server invariant)
3. Global disconnect from settings → all BP connections cleared; panels show "not connected"

## Migration

No action required.
