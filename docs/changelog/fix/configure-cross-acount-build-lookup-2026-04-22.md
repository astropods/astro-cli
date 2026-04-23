# Configure prefill resolves build under the deployment's source account

## Summary

Opening the Configure panel for a cross-account deployment returned `build not found` and a 404 from the deployment-template endpoint, even though the agent was deployed and running healthily. The deployment's workspace (URL `:account`) and the publisher that actually owns the agent/build were different accounts, and the handler was resolving the build under the URL account instead of the publisher recorded in the deployment spec.

## Design

Cross-account deployments already persist the publisher in `deployment_spec_json.source.account`; the reconcile worker reads this field when it needs to look up the agent. The prefill path in `PostDeploymentTemplate` did not, so once a deployment was opened under a workspace that was not the publisher's account, `agentIndex.GetVersion` was called with the wrong `account_id` and the lookup failed.

### Shared source-account resolution

The ad-hoc parser inside `internal/riverqueue/reconcile.go` is lifted into an exported helper on the store package so every caller reads `source.account` the same way:

```go
// internal/deploymentstore/spec.go
func SourceAccountFromSpec(specJSON string) string
```

Returns empty for empty specs, missing `source`, or parse failures. Callers treat empty as "no publisher recorded" and fall back to their prior behavior. `reconcile.go` now consumes this helper and drops its local copy.

### Prefill uses the publisher for the build lookup

`generateTemplate` in `handlers/deploy.go` gains two new parameters:

- `sourceAccountOverride string` — when non-empty, replaces `c.Param("account")` for the agent/version lookup.
- `skipVisibilityCheck bool` — skips the private-agent membership recheck inside `generateTemplate`.

`PostDeploymentTemplate`'s prefill branch (`deployment_id` present) now:

1. Reads `deployment_spec_json.source.account` via `SourceAccountFromSpec` (using the historical revision's spec when a revision is requested).
2. Passes that value as `sourceAccountOverride` so the agent and build are looked up under the publisher.
3. Authorizes once against the deployment's target account — the same check that already gates the prefill branch — and passes `skipVisibilityCheck=true` so `generateTemplate` does not re-run a private-agent membership check against the publisher account. Re-running it would start requiring source-account membership for every cross-account Configure, which would break the publishing model whenever a publisher's agent is private.
4. The fresh-template branch (no `deployment_id`) is unchanged: it still uses the URL account and keeps the visibility check on.

The cache key for the prefill branch is extended with `sourceAccountName` so two deployments with the same `(account, agent, build, deployment_id, revision)` but different publishers never collide.

### Pinning semantics preserved

Build selection is untouched: Configure continues to use `existing.BuildID` via the existing `GetVersion(account, name, buildID)` path. The change is purely which account that triple resolves against.

### Tests

Two test layers pin the behavior without coupling to internals.

`apps/astro-server/handlers/deploy_test.go` (sqlmock-based handler tests):

- Cross-account prefill resolves the agent and build under the publisher named in `source.account`.
- Same-account prefill behaves identically to the legacy path.
- Legacy prefill (spec has no `source`) falls back to the URL account.
- Cross-account auth stays scoped to the deployment's target account — the mock queue seeds exactly one `IsMember` query against that account; any extra `IsMember` against the source account would fail the test.
- Cross-account pinning returns the deployed build even when the source account has a newer one — only the 3-arg `GetVersion` is mocked, so a fall-through to `GetLatestVersion` would fail.

`apps/astro-server/e2e/configure_cross_account_test.go` (integration tests, real Postgres, `//go:build integration`):

- Same five scenarios driven end-to-end through the HTTP handler with real account/agent/deployment rows.
- Runs in CI under the dedicated `test-go-integration` job; not picked up by the local `moon run astro-server:e2e` target, which uses the `k8s` tag.

## Migration

None. Existing deployments continue to work. Cross-account deployments whose Configure panel previously returned `build not found` will resolve correctly on the next open; no backfill or spec rewrite is required.
