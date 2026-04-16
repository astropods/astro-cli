## Summary

This change completes the GitHub onboarding feature by restoring two pieces of functionality that were dropped during a rebase, fixing cache invalidation bugs, and adding comprehensive test coverage.

## Design

### Disable already-linked repos in the wizard

When a user opens the "Set up with GitHub" path in the new blueprint wizard, the repo picker now fetches the account's existing GitHub connections (`GET /api/v1/accounts/:account/github/connections`) and disables any repo that is already linked to another blueprint, with a tooltip showing which blueprint it's linked to.

This required a new backend endpoint backed by `githubconnection.Store.ListByAccount` and a new `useGitHubAccountConnections` TanStack Query hook.

### Disconnect GitHub repo on archive

When a blueprint is archived, the server now fires a best-effort goroutine to delete the GitHub webhook and remove the `github_connections` row. This frees the repo to be linked to a new blueprint immediately, without a manual disconnect step.

The goroutine is nil-guarded so existing tests that pass `nil` for `ghStore` are unaffected.

### Cache invalidation

The `useArchiveBlueprint` mutation now invalidates both `githubKeys.accountConnections(account)` and `githubKeys.status(account, name)` in `onSuccess`, so the wizard's repo picker reflects the updated connection state immediately after an archive — no page refresh needed.

### Test coverage

- **Go**: Nil-guard fix for the disconnect goroutine; `visibility_test.go` updated for the new `ArchiveAgent` signature; `client_test.go` updated to include `Permissions.Admin: true` so repos pass the admin filter.
- **E2E — existing tests fixed**: `blueprint-creation.spec.ts` and `onboarding-flow.spec.ts` updated to navigate the multi-step wizard (identity step → Continue → source step → Create blueprint).
- **E2E — new tests**: `github-onboarding.spec.ts` adds 4 flows: local setup, GitHub import, linked-repo disabled, and archive-releases-repo.
- **Mock backend**: GitHub endpoints added (`connect`, `repos`, `connections`, `scan`, `link`, `unlink`, `status`); archive handler clears connections in mock state.

## Migration

No user action required.
