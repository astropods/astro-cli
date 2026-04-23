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

`generateTemplate` in `handlers/deploy.go` is refactored to take a pre-resolved `(*account.Account, *agentindex.Agent)` pair and generate the template from those. Each call site in `PostDeploymentTemplate` now resolves its own account + agent up front via a new `resolveAgentForTemplate` helper and decides explicitly whether to run the private-agent visibility gate (`enforcePrivateAgentMembership`). This replaces the previous design where `generateTemplate` did the lookups and an opaque boolean controlled whether it also ran the membership check.

`PostDeploymentTemplate`'s prefill branch (`deployment_id` present) now:

1. Reads `deployment_spec_json.source.account` via `SourceAccountFromSpec` (using the historical revision's spec when a revision is requested), falling back to the URL account when the field is absent.
2. Resolves the agent + account under that publisher and calls `generateTemplate` — no source-agent visibility check.
3. Keeps authorization scoped to the deployment's target account, the same check that already gated the prefill branch. Requiring source-account membership here would break cross-account Configure for any private agent that had already been published and deployed downstream.

The fresh-template branch (no `deployment_id`) resolves the agent under the URL account, runs `enforcePrivateAgentMembership`, then calls `generateTemplate`. Its observable behavior is unchanged.

The cache key for the prefill branch is extended with `sourceAccountName`, placed directly after the URL account for readability (`accountName:sourceAccount:agentName:build:deployment:revision`), so two deployments with the same `(account, agent, build, deployment_id, revision)` but different publishers never collide.

`source.account` lives in `deployment_spec_json`, not on a dedicated column of the `deployments` table. The only existing `source_account` column lives on `namespace_ownership` and is a derived mirror that the reconcile worker writes from the same JSON field. Promoting it to a first-class column on `deployments` is worth doing but is out of scope here; parsing the JSON is what `reconcile` already does and is now centralized via `SourceAccountFromSpec`.

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
