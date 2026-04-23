# Configure prefill resolves build under the deployment's source account

## Summary

Opening the Configure panel for a cross-account deployment returned `build not found` and a 404 from the deployment-template endpoint, even though the agent was deployed and running healthily. The deployment's workspace (URL `:account`) and the publisher that actually owns the agent/build were different accounts, and the handler was resolving the build under the URL account instead of the publisher recorded in the deployment spec.

## Design

The publisher was already persisted inside `deployment_spec_json.source.account`, but only the reconcile worker was reading it, and only to derive the cached `source_account` column on `namespace_ownership`. The prefill path in `PostDeploymentTemplate` did not look at it at all, so once a deployment was opened under a workspace that was not the publisher's account, `agentIndex.GetVersion` was called with the wrong `account_id` and the lookup failed.

### First-class `source_account_id` on `deployments`

The schema promotes the publisher from a JSON field to a nullable foreign-key column:

```sql
ALTER TABLE deployments
  ADD COLUMN source_account_id uuid
  REFERENCES accounts(id) ON DELETE SET NULL;
```

Writes in `SaveDeploymentPending` and `UpdateDeploymentPending` populate the column from the deploy request's resolved source account; same-account deployments store the publisher too so the column is always populated going forward. Reads in `PostDeploymentTemplate` (and elsewhere that needs the publisher) prefer the column and only fall back to `SourceAccountFromSpec(deployment_spec_json)` for un-backfilled rows. A helper `resolveSourceAccountName(store, deployment)` encapsulates the column-first-then-JSON resolution.

An idempotent backfill runs once at server startup for any row where `source_account_id IS NULL`: parse `source.account` out of the spec, look up the account id by name, and fall back to the deployment's own `account_id` when the spec has no publisher or the name cannot be resolved. Startup logs the counts. Re-running is a no-op.

`namespace_ownership.source_account` is now a derived cache of the same information and can be dropped in a follow-up; it is left in place here to keep this change focused on the read path that was broken.

### Prefill uses the publisher for the build lookup

`generateTemplate` in `handlers/deploy.go` is refactored to take a pre-resolved `(*account.Account, *agentindex.Agent)` pair and generate the template from those. Each call site in `PostDeploymentTemplate` now resolves its own account + agent up front via a new `resolveAgentForTemplate` helper and decides explicitly whether to run the private-agent visibility gate (`enforcePrivateAgentMembership`). This replaces the previous design where `generateTemplate` did the lookups and an opaque boolean controlled whether it also ran the membership check.

`PostDeploymentTemplate`'s prefill branch (`deployment_id` present) now:

1. Resolves the publisher via `resolveSourceAccountName` (column first, spec JSON as fallback), falling back to the URL account when neither is present.
2. Resolves the agent + account under that publisher and calls `generateTemplate` — no source-agent visibility check.
3. Keeps authorization scoped to the deployment's target account, the same check that already gated the prefill branch. Requiring source-account membership here would break cross-account Configure for any private agent that had already been published and deployed downstream.

The fresh-template branch (no `deployment_id`) resolves the agent under the URL account, runs `enforcePrivateAgentMembership`, then calls `generateTemplate`. Its observable behavior is unchanged.

The cache key for the prefill branch is extended with `sourceAccountName`, placed directly after the URL account (`accountName:sourceAccount:agentName:build:deployment:revision`), so two deployments with the same `(account, agent, build, deployment_id, revision)` but different publishers never collide.

### Pinning semantics preserved

Build selection is untouched: Configure continues to use `existing.BuildID` via the existing `GetVersion(account, name, buildID)` path. The change is purely which account that triple resolves against.

### Tests

Two test layers pin the behavior without coupling to internals.

`apps/astro-server/handlers/deploy_test.go` (sqlmock-based handler tests):

- Cross-account prefill resolves the agent and build under the publisher named in `source.account` / `source_account_id`.
- Same-account prefill behaves identically to the legacy path.
- Legacy prefill (column null, spec has no `source`) falls back to the URL account.
- Cross-account auth stays scoped to the deployment's target account — the mock queue seeds exactly one `IsMember` query against that account; any extra `IsMember` against the source account would fail the test.
- Cross-account pinning returns the deployed build even when the source account has a newer one — only the 3-arg `GetVersion` is mocked, so a fall-through to `GetLatestVersion` would fail.

`apps/astro-server/e2e/configure_cross_account_test.go` (integration tests, real Postgres, `//go:build integration`):

- Same scenarios driven end-to-end through the HTTP handler with real account/agent/deployment rows.
- Runs in CI under the dedicated `test-go-integration` job; not picked up by the local `moon run astro-server:e2e` target, which uses the `k8s` tag.
- The fixture writes deployments through `SaveDeploymentPending` and reads them back through `PostDeploymentTemplate`, so the new column is exercised on both the write and read path. A NULL-column path is covered by the legacy case where the spec has no `source` block.

`apps/astro-server/e2e/backfill_source_account_id_test.go` (integration tests):

- Covers every branch of `BackfillSourceAccountIDs`: resolve from spec, fall back when the spec name does not match an account, fall back when `source` is absent, fall back on malformed JSON.
- Asserts the WHERE `source_account_id IS NULL` guard: rows already populated by new-code writes are not clobbered even when the spec JSON would point elsewhere.
- Asserts re-running the backfill is a no-op, which matters because it runs async at every server start.

## Migration

The schema change is additive (new nullable column + foreign key). The backfill runs automatically in the background on API startup and is idempotent.

Rollout order matters in one direction only: the declarative schema apply must run before the new server binary. Old code tolerates the new column (all SELECTs use explicit column lists), but new code fails against the old schema (`scanDeployment` expects `source_account_id`). Apply `sql-migrate.yml` first, then deploy the server. Reverting the code alone is safe; dropping the column is not required.

Cross-account deployments whose Configure panel previously returned `build not found` will resolve correctly on the next open.
