# Account Deletion Implementation Plan

## Context

The `DELETE /api/v1/accounts/:account` endpoint currently returns 501 Not Implemented. The frontend (`DeleteAccountDialog`) already has the full UI flow — confirmation dialog, mutation hook, logout on success. We need to implement the backend handler and add safety filters to prevent deleted accounts from being accessible.

## Approach: Synchronous soft-delete + async deployment teardown

Soft-delete the account immediately (sets `deleted_at`), enqueue undeploy jobs for all active deployments, clean up external services best-effort, return 200. The user logs out and never sees the account again. K8s teardown happens in the background via the existing River queue undeploy pipeline.

**Why soft-delete:** Reversible, immediate from the user's perspective, no CASCADE timing issues. A future background job can hard-delete after a retention period.

## Changes

### 1. `apps/astro-server/handlers/accounts.go` — Rewrite `DeleteAccount`

Expand the function signature to accept the dependencies it needs:

```go
func DeleteAccount(
    log *logger.Logger,
    accountStore *account.AccountStore,
    deployStore *deploymentstore.Store,
    queue DeployQueue,
    orgClient *org.Client,
    db *sql.DB,
) gin.HandlerFunc
```

Handler body (in order):
1. Get account from context (existing)
2. `accountStore.MarkDeleted(acct.ID)` — point of no return. Return 500 on error. If already deleted, return 404.
3. `deployStore.GetVisibleDeploymentsByAccount(acct.ID)` — for each deployment, call `deployStore.UpdateStatus(dep.ID, "undeploying", "", nil)` then `queue.InsertUndeployJob(ctx, dep.ID)`. Log failures but continue.
4. If `acct.WorkOSOrganizationID != ""` and `orgClient != nil`, call `orgClient.DeleteOrganization(ctx, acct.WorkOSOrganizationID)`. Log-and-continue on failure.
5. `db.ExecContext(ctx, "DELETE FROM agent_message_counts WHERE account_id = $1", acct.ID)`. Log-and-continue.
6. Return 200 `{"message": "account deleted"}`.

### 2. `apps/astro-server/main.go:619` — Update route wiring

Change `handlers.DeleteAccount(log, accountStore)` to:
```go
handlers.DeleteAccount(log, accountStore, deploymentStore, queue, orgClient, db)
```

All variables already in scope in `setupRoutes`. Also remove the `501` response spec, keep `200` and add `500`.

### 3. `apps/astro-server/internal/account/store.go` — Filter deleted accounts

Add `AND a.deleted_at IS NULL` to three queries:
- **`GetByName`** (line 109) — used by `ResolveAccount` middleware. Without this, deleted accounts remain accessible via API.
- **`GetByID`** (line 126) — same concern.
- **`GetAccountsForUser`** (line 174) — without this, deleted accounts still appear in the user's account list.

### 4. No frontend changes needed

The frontend already handles success (logout + navigate to "/") and error (shows message in dialog). Switching from 501 to 200 on success requires zero code changes.

## What we skip (and why)

- **OpenMeter customer deletion** — no `DeleteCustomer` API method exists. Orphaned customer is harmless once no events flow. Add a TODO.
- **Langfuse external project** — DB credentials cascade, external project is independent. Leave alone.
- **Hard delete** — defer to a future retention/purge job. Soft delete is sufficient.

## Edge cases

- **Already deleted account:** `MarkDeleted` checks `deleted_at IS NULL`, returns error → handler returns 404.
- **No deployments:** Loop does nothing. Common case.
- **Partial undeploy failure:** Existing reconciler catches orphaned namespaces.
- **orgClient nil:** Normal when WorkOS not configured. Skip that step.
- **Race with concurrent requests:** After `MarkDeleted`, the `GetByName` filter ensures the middleware rejects subsequent requests with 404.

## Verification

1. `cd apps/astro-server && go build ./...` — confirm compilation
2. `moon run astro-server:test` — existing tests pass
3. Write a unit test for the handler: mock stores/queue, verify MarkDeleted called, undeploy jobs enqueued, WorkOS org deleted, 200 returned
4. Test partial failure: mock one undeploy failing, verify 200 still returned
5. Test already-deleted: verify 404

## Files to modify

- `apps/astro-server/handlers/accounts.go` (rewrite DeleteAccount)
- `apps/astro-server/main.go` (update route wiring at line 619)
- `apps/astro-server/internal/account/store.go` (add deleted_at filters to GetByName, GetByID, GetAccountsForUser)
