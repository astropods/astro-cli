# fix(github): surface stale token as reconnect prompt instead of 500

## Summary

When a user browses GitHub repos in the blueprint page, the server fetches a GitHub OAuth token from WorkOS Pipes. Intermittently in production, WorkOS returns a cached token that GitHub has already revoked. Previously this caused a 500 server error with no actionable path for the user. Now it surfaces as a reconnect prompt.

## Design

The GitHub REST client (`internal/github/client.go`) wraps all 401 responses from GitHub's API with a typed `ErrUnauthorized` sentinel error. Callers can detect this with `errors.Is` without string-matching the error message.

The `GitHubAccountListRepos` handler checks for `ErrUnauthorized` on both the orgs lookup and the repo search. When detected it returns `422 github_not_connected` — the same response the handler already returns when WorkOS itself reports no token — so the frontend shows the user a reconnect prompt rather than a generic error.

The root cause (WorkOS returning a revoked cached token) is a WorkOS Pipes issue and requires a fix on their end.

### Orgs cache invalidation on reconnect

The repo search handler caches a user's GitHub org memberships for 5 minutes. If a token is revoked for any reason and the user reconnects — potentially with a different GitHub account — the cached org list from the previous session could persist and be used in repo searches with the new token.

`GitHubAccountCallback` (the OAuth landing endpoint WorkOS redirects to after the user approves access) now evicts the orgs cache entry once the new token is confirmed, so the next repo search always fetches fresh org memberships.

## Migration

No action required.
