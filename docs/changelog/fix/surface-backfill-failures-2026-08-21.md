# Make a failed startup backfill rarer and visible

## Summary

Two backfills run on the deployment controller's leader: one records the primary cluster on deployments that route there, the other binds every account to its primary cluster. Each runs once per leadership term, so a failure does not retry until the next leader election, and each leaves a reader consuming state that was never established. A failed binding backfill is the more costly of the two: the registry refuses image pulls for an account with no bindings, so pods fail with ImagePullBackOff until someone restarts the leader.

Both failed quietly, at warning level. The binding backfill also failed more often than it needed to.

## Design

**One deleted account no longer aborts the batch.** The binding backfill is a single `INSERT ... SELECT` over every unbound account. Under read committed the select takes its snapshot when the statement starts, while the foreign key check runs against the current state, so an account hard-deleted and committed in between is returned by the select and gone by the check. The statement then fails with a foreign key violation and nothing is bound at all.

The select now takes `FOR KEY SHARE` on each account it reads. Postgres re-checks a locked row against its latest version, so a row deleted by a committed transaction is skipped rather than returned stale, and a delete still in flight waits until the insert commits. `KEY SHARE` is the lock the foreign key check takes anyway: it blocks deletes of the accounts being bound and nothing else, for the length of one pass.

**A failure reaches whoever is watching.** Both backfills log at error level, and each message names what breaks and when it is retried, so the alert does not have to be decoded against the source.

Retrying inside the leadership term would remove the wait entirely. Both statements are idempotent, so that remains available if failures turn out to be common enough to warrant it.

## Migration

None.
