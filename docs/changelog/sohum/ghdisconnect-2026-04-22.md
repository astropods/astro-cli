# GitHub Account Connect / Disconnect

## Summary

The GitHub OAuth integration had several broken seams: the account-level disconnect toggle would flip back on immediately after clicking, the WorkOS Pipes token was never revoked on disconnect (so `GET /github` kept returning `connected: true`), the OAuth redirect path landed on a 404, and the BP panel never showed the connected username when a user had OAuth'd but not yet picked a repo. This change fixes all of them as a coherent system.

## Design

### Connection state ownership

GitHub connection state now lives in two layers that must stay in sync:

- **WorkOS Pipes** — holds the OAuth token. `GET /api/v1/accounts/:account/github` checks for a live token; `DELETE` revokes it via `pipes.DeleteConnection` (calls `DELETE /user_management/users/:user_id/connected_accounts/:provider`).
- **`github_connections` table** — holds per-agent repo links. Account disconnect iterates all rows for the account, does best-effort webhook removal per row, then deletes all rows before revoking the token.

Revoking the token is the last step so a partial failure (e.g. webhook removal fails) doesn't leave the account in a state where it looks disconnected but the token is still live.

### Client cache strategy

The `accountStatus` cache entry (`githubKeys.accountStatus(account)`) is the single source of truth for the connected toggle in Account Settings. Three things write to it:

| Trigger | Write |
|---|---|
| Disconnect mutation `onSuccess` | `cancelQueries` + `setQueryData({ connected: false })` — cancel prevents an in-flight refetch from racing and flipping the toggle back |
| Connect mutation `onSuccess` (direct-connect path) | `setQueryData({ connected: true, github_login })` — avoids an extra round-trip to display the login |
| OAuth redirect (page reload) | Cache is cleared; background fetch after `initialData` seeds it from URL params |

The `initialData` + `initialDataUpdatedAt: 0` pattern handles the OAuth redirect case: the query is seeded synchronously from `?github_connected=true&github_login=…` params so the UI shows "connected" immediately, then `initialDataUpdatedAt: 0` marks it stale and triggers a background refetch to confirm the token is live. The `useEffect` is reduced to cleanup-only (removing the query params from the URL).

`setQueryData` for the accountStatus cache was previously duplicated across `GitHubConnectionPanel` and `AccountSettings`. It's now consolidated in `useGitHubAccountConnect.onSuccess` — the one canonical place.

### BP panel username fallback

`githubLogin` in the panel header uses a three-level fallback:

1. `status.repo_full_name.split('/')[0]` — owner from the linked repo (agent is connected + linked)
2. `effectiveRepo.split('/')[0]` — owner from a wizard-supplied pre-connection repo
3. `accountStatus.github_login` — from the account-level OAuth token (connected but no repo yet)

This ensures the panel header shows a username during the window between completing OAuth and picking a repo.

### OAuth redirect path fix

The `redirectTo` value was `/${account}/settings/account` (wrong — settings routes have no account prefix) and included `?github_connected=true` as a suffix (causing a double-`?` when the server appended the same param). Fixed to `/settings/account`.

### Token revocation (server)

`pipes.DeleteConnection` is a new method on `*pipes.Client`. It calls the WorkOS API directly (not in the Go SDK yet): `DELETE /user_management/users/:user_id/connected_accounts/:provider`. On non-2xx it returns an error with the status code and body. In `GitHubAccountDisconnect` the call is best-effort — a WorkOS outage won't block users from disconnecting, it just logs a warning.

## Tests

- **`handlers/github_account_test.go`** — handler-level tests for `GitHubWebhook` (6 cases: non-push, invalid JSON, unknown repo, bad HMAC, wrong branch, branch deletion), `verifyGitHubSignature` (5 table-driven cases), `firstCommitLine` (5 cases), and unauthenticated paths for the two new account endpoints.
- **`internal/pipes/client_test.go`** — `DeleteConnection`: success, org_id query param, 400 error, 500 error.
- **`api/queries/github.test.ts`** — `useGitHubAccountStatus` (6 cases), `useGitHubAccountDisconnect` (3 cases: DELETE call, cache write, invalidation), `useGitHubAccountConnect` (2 cases: connected path vs redirect path).
- **`GitHubConnectionPanel.test.tsx`** — connected-account-no-repo state, direct connect (no OAuth) path.
- **`e2e/github-connection-flow.spec.ts`** — three end-to-end flows against a mock backend: link repo → settings shows connected; unlink BP repo → account connection survives; global disconnect → all BP panels show not-connected.

## Migration

No action required.
