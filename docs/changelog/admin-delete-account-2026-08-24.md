# Delete and purge an account from the admin console

## Summary

Only an account's own owner could delete it. When an account went defunct, an abandoned org, a spam signup, a test account whose owner had lost access, nobody could remove it. The admin console could rename the account, rebind its clusters, and repair its billing, but not delete it, so the row and its name stayed occupied indefinitely.

Deleting was also only half the lifecycle. Delete soft-deletes; a periodic sweep hard-deletes once the retention window passes. The sweep skips an account it cannot finish and returns success anyway, so an account permanently blocked on teardown produced an unbroken series of healthy hourly jobs and no other signal. Purge status was tracked nowhere: it was inferred from whether the row still existed.

The admin console can now delete an account, purge one ahead of its retention window, and see the accounts whose purge is overdue.

## Design

### One delete sequence

The delete sequence moved out of the `DeleteAccount` HTTP handler into `internal/accountlifecycle`. `Deleter.Delete` archives the billing customer, soft-deletes the account, revokes the account-scoped AI Gateway judge key, queues every visible deployment for teardown, and removes the WorkOS organization. Both the owner-facing route and the admin RPC call it.

Ordering carries the invariant: billing is archived first, and a failure there aborts before anything is mutated. An account marked deleted while its customer keeps accruing charges bills someone nobody is watching, and that is the only step whose failure still costs money afterward. Everything past the soft-delete is best-effort, because the purge worker retries each step before it removes the row.

`Undeploy` is a function field rather than an inlined store call, so this package and the public undeploy route share `handlers.EnqueueUndeploy` instead of each carrying its own copy of "set undeploying, then enqueue". The same shape already backs the purge worker.

### The admin surface

`AdminService.DeleteAccount` takes the account UUID plus a `confirm_name` that must equal the account's current name. The typed name is the only guard that distinguishes the intended account from another: a UUID transposed from a neighboring row is just as well-formed as the right one. The response reports how many deployments were queued for teardown.

The RPC reaches the server through `DELETE /api/admin/accounts/{id}?confirm_name=...`. The confirmation travels in the query string because the console's fetch wrapper does not carry DELETE bodies. Every delete writes an `account.delete` audit event attributed to the admin channel.

The account detail page's danger zone gains a delete action behind a dialog that requires the account name typed in full. The page states what the delete does and that the purge job hard-deletes the row and frees the name after the retention window.

### One purge sequence

The hard-delete moved out of `AccountPurgeWorker` into `accountlifecycle.Purger` alongside the deleter. `Purger.Purge` deletes the Langfuse project, revokes the remaining gateway keys, and hard-deletes the row; `Purger.Overdue` returns the accounts past retention. The worker is now the sweep and nothing else: resolve retention, list, loop, count.

The worker owns the only fully-wired `Purger`, because it assembles collaborators the rest of the process does not have (the Langfuse provisioner and gateway stores come from the deployer). Rather than building a second set for the admin path, the queue exposes its own through `Queue.AccountPurger()`, so an on-demand purge and a scheduled one cannot clean up different things.

`Purge` returns `ErrTeardownPending` when deployments have not finished undeploying or authorization rows have not converged. It re-enqueues the undeploys it is waiting on, then refuses. Distinguishing that from a real failure is what lets the admin RPC answer "not yet, waiting on 2 deployments" instead of "internal error".

### Purging on demand

`AdminService.PurgeAccount` runs the same sequence for one account with no retention check, reachable at `POST /api/admin/accounts/{id}/purge?confirm_name=...`. It requires the account to already be soft-deleted: purging a live account would skip the billing archive and the WorkOS organization delete that only the soft-delete performs. `ErrTeardownPending` maps to `FailedPrecondition` carrying the blocker, so the operator sees what to fix.

The account detail page shows a purge panel in place of the danger zone once an account is deleted, behind the same typed-name confirmation.

### Surfacing an overdue purge

A new system audit check, `account.purge_overdue`, reports accounts still present more than a day past their retention window. The grace day keeps the hourly sweep's normal lag out of the results, so a finding means the purge has failed roughly two dozen times, not that it has not run yet.

The finding's detail carries the diagnosis rather than just the fact: `pending_deployments`, `pending_authorization`, `has_langfuse_project`, and `days_deleted`. Those are the blockers `Purge` refuses on, so the finding says which one is holding the account. The findings page already routes `account.*` subjects to the account detail page, which is where the purge action now lives.

### Retention is one constant

`accountlifecycle.RetentionDays` is now the single source for the window: the sweep derives its cutoff from it and the check's interval is built from it, so the two cannot disagree about when an account is overdue.

It replaces `riverqueue.Config.AccountRetentionDays`, which was declared and read but never assigned anywhere, so the worker's `if retentionDays <= 0` fallback was the effective window in every environment. The field promised configurability that did not exist. Purging on demand is the answer to "this one needs to go sooner", which is what the field would have been reached for, so the knob is gone rather than wired up.

## Migration

None. The owner-facing delete behaves identically. `handlers.DeleteAccount` now takes a `*accountlifecycle.Deleter` in place of the stores and clients it used to thread through, an internal signature change with no effect on the route.
