# Account Deletion Implementation Plan

## Context

The `DELETE /api/v1/accounts/:account` endpoint currently returns 501 Not Implemented. The frontend (`DeleteAccountDialog`) already has the full UI flow — confirmation dialog, mutation hook, logout on success. We need to implement the backend handler and add safety filters to prevent deleted accounts from being accessible.

## Approach: Synchronous soft-delete + async deployment teardown

Soft-delete the account immediately (sets `deleted_at`), enqueue undeploy jobs for all active deployments, clean up external services best-effort, return 200. The user logs out and never sees the account again. K8s teardown happens in the background via the existing River queue undeploy pipeline.

**Why soft-delete:** Reversible, immediate from the user's perspective, no CASCADE timing issues. A future background job can hard-delete after a retention period.

## Changes

### 1. `apps/astro-server/handlers/deploy.go` — Extract `EnqueueUndeploy` helper

Extract the shared undeploy core (currently lines 571-580 in `UndeployAgent`) into a package-level helper:

```go
func EnqueueUndeploy(ctx context.Context, deployStore *deploymentstore.Store, queue DeployQueue, deploymentID string) error
```

Sets status to `undeploying` via `deployStore.UpdateStatus`, then calls `queue.InsertUndeployJob`. Returns first error. The River `UndeployWorker` handles K8s namespace teardown, status transition to `undeployed`, and `ClearScaledDown` asynchronously (with 3 retries).

Refactor `UndeployAgent` to call `EnqueueUndeploy` instead of inlining those two calls.

### 2. `apps/astro-server/handlers/accounts.go` — Rewrite `DeleteAccount`

Expand the function signature to accept the dependencies it needs:

```go
func DeleteAccount(
    log *logger.Logger,
    accountStore *account.AccountStore,
    deployStore *deploymentstore.Store,
    queue DeployQueue,
    orgClient *org.Client,
) gin.HandlerFunc
```

Handler body (in order):
1. Get account from context (existing)
2. `accountStore.MarkDeleted(acct.ID)` — point of no return. Return 500 on error. If already deleted, return 404.
3. `deployStore.GetVisibleDeploymentsByAccount(acct.ID)` — for each deployment, call `EnqueueUndeploy(ctx, deployStore, queue, dep.ID)`. Log failures but continue.
4. If `acct.WorkOSOrganizationID != ""` and `orgClient != nil`, call `orgClient.DeleteOrganization(ctx, acct.WorkOSOrganizationID)`. Log-and-continue on failure.
5. Return 200 `{"message": "account deleted"}`.

No manual `agent_message_counts` cleanup needed — FK now cascades from `accounts`.

### 3. `apps/astro-server/main.go:619` — Update route wiring

Change `handlers.DeleteAccount(log, accountStore)` to:
```go
handlers.DeleteAccount(log, accountStore, deploymentStore, queue, orgClient)
```

All variables already in scope in `setupRoutes`. Also remove the `501` response spec, keep `200` and add `500`.

### 4. `apps/astro-server/internal/account/store.go` — Filter deleted accounts

Add `AND a.deleted_at IS NULL` to three queries:
- **`GetByName`** (line 109) — used by `ResolveAccount` middleware. Without this, deleted accounts remain accessible via API.
- **`GetByID`** (line 126) — same concern.
- **`GetAccountsForUser`** (line 174) — without this, deleted accounts still appear in the user's account list.

### 5. No frontend changes needed

The frontend already handles success (logout + navigate to "/") and error (shows message in dialog). Switching from 501 to 200 on success requires zero code changes.

### 6. `apps/astro-server/internal/riverqueue/` — Add `AccountPurge` periodic job + worker

A periodic River job that finds accounts past the retention period and hard-deletes all associated data.

**`purge_accounts.go`** — Job args + worker:

```go
type AccountPurgeArgs struct{}
func (AccountPurgeArgs) Kind() string { return "account.purge" }
```

Worker runs on the default queue. On each tick:
1. Query `SELECT id FROM accounts WHERE deleted_at IS NOT NULL AND deleted_at < NOW() - INTERVAL '7 days'` (configurable retention via `Config.AccountRetentionDays`, default 7).
2. For each account, in a single transaction:
   - **Retry failed teardowns:** Query deployments still in `undeploying` or `active`/`failed` status for this account. Re-enqueue undeploy jobs for any that haven't reached `undeployed`. Skip hard-delete for this account if any deployments are still not `undeployed` — the next tick will retry.
   - **Hard-delete:** `DELETE FROM accounts WHERE id = $1` — all child tables now cascade (`deployments`, `namespace_ownership`, `connected_devices`, `agent_message_counts`, `agents`, `account_members`, `account_langfuse`, etc.).
   - **Clean up external resources** (before hard-delete, outside tx — if any fail, skip this account and retry next tick):
     - **Billing:** call `DeleteCustomer(ctx, customerID)` on the configured `BillingProvider` if the account has a stored billing customer ID. Treat not-found as success (already gone).
     - **Langfuse:** Read `account_langfuse` row before deleting it. Use the Langfuse project ID to call the Langfuse API `DELETE /api/public/projects/{projectId}` (using the stored public/secret key for auth). Treat 404 as success. Add a `DeleteProject` method to the langfuse package.
3. Log summary: accounts purged, accounts skipped (pending teardown or external cleanup failure), errors.

**`periodic.go`** — Register the job:
```go
river.NewPeriodicJob(
    river.PeriodicInterval(1*time.Hour),
    func() (river.JobArgs, *river.InsertOpts) {
        return AccountPurgeArgs{}, &river.InsertOpts{
            UniqueOpts: river.UniqueOpts{ByPeriod: 1 * time.Hour},
        }
    },
    &river.PeriodicJobOpts{RunOnStart: true},
)
```

**`workers.go`** — Register `AccountPurgeWorker`.

**`client.go`** — Add `AccountRetentionDays int` to `Config` (default 30).

## What we skip (and why)

- **Hard delete at request time** — deferred to the purge job after the retention period. Soft delete is sufficient for immediate UX.

## Edge cases

- **Already deleted account:** `MarkDeleted` checks `deleted_at IS NULL`, returns error → handler returns 404.
- **No deployments:** Loop does nothing. Common case.
- **Partial undeploy failure:** Purge job retries unfinished teardowns on each tick; existing reconciler also catches orphaned namespaces.
- **orgClient nil:** Normal when WorkOS not configured. Skip that step.
- **Race with concurrent requests:** After `MarkDeleted`, the `GetByName` filter ensures the middleware rejects subsequent requests with 404.

## Verification

1. `cd apps/astro-server && go build ./...` — confirm compilation
2. `moon run astro-server:test` — existing tests pass
3. Write a unit test for the handler: mock stores/queue, verify MarkDeleted called, undeploy jobs enqueued, WorkOS org deleted, 200 returned
4. Test partial failure: mock one undeploy failing, verify 200 still returned
5. Test already-deleted: verify 404
6. Test purge worker: mock an account with `deleted_at` older than retention, verify all tables cleaned up
7. Test purge skips accounts with pending teardowns: mock a deployment still in `undeploying`, verify purge re-enqueues undeploy and skips hard-delete

## Files to modify

- `apps/astro-server/handlers/deploy.go` (extract `EnqueueUndeploy` helper, refactor `UndeployAgent` to use it)
- `apps/astro-server/handlers/accounts.go` (rewrite `DeleteAccount` to use `EnqueueUndeploy`)
- `apps/astro-server/main.go` (update route wiring at line 619)
- `apps/astro-server/internal/account/store.go` (add deleted_at filters to GetByName, GetByID, GetAccountsForUser)
- `apps/astro-server/internal/billing` (`DeleteCustomer` on the `BillingProvider` interface)
- `apps/astro-server/internal/langfuse/` (add `DeleteProject` method or standalone client func)
- `apps/astro-server/internal/riverqueue/purge_accounts.go` (new — `AccountPurgeArgs` + `AccountPurgeWorker`)
- `apps/astro-server/internal/riverqueue/periodic.go` (register purge periodic job)
- `apps/astro-server/internal/riverqueue/workers.go` (register `AccountPurgeWorker`)
- `apps/astro-server/internal/riverqueue/client.go` (add `AccountRetentionDays` to `Config`)
