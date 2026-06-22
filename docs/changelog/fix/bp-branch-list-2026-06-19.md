# Blueprint create: list real repo branches

## Summary

The branch selector on the blueprint create flow offered a hardcoded
`main` / `master` (plus the repo's default branch). It never reflected the
branches that actually exist on the connected repo, so users picking any other
branch had no way to select it and could pick a branch that doesn't exist.

## Design

Branches are now fetched from GitHub for the selected repo, end to end:

- **Server** — a new `ListBranches` GitHub client call (`GET /repos/{repo}/branches`,
  one page of up to 100) backs a new account-scoped endpoint
  `GET /accounts/:account/github/branches?repo=owner/name`. Auth and token
  retrieval mirror the existing repo/orgs list handlers.
- **Client** — a `useGitHubAccountBranches` query hook fetches branches once a
  repo is selected. The picker surfaces the repo's default branch first, then the
  remaining branches (deduped), and shows a spinner while the list loads. If the
  fetch is in flight or fails, it falls back to the repo's default branch so the
  selector always has a valid value.

Branch lists are capped at one page (100). Repos with more branches are
truncated; the default branch is always present.

## Migration

None. No API or configuration changes are required.
