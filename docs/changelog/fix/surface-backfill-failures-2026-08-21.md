# Surface a failed startup backfill

## Summary

Two backfills run on the deployment controller's leader: one records the primary cluster on deployments that route there, the other binds every account to its primary cluster. Both log at warning level and return when they fail.

Each runs once per leadership term, so a failure does not retry until the next leader election, and each leaves a reader consuming state that was never established. A failed binding backfill is the more costly of the two: the registry refuses image pulls for an account with no bindings, so pods fail with ImagePullBackOff until someone notices and restarts the leader.

## Design

Both failures log at error level, and each message names what breaks and when it is retried, so whoever reads the alert does not have to reconstruct either from the code. Nothing else changes: the passes are already idempotent, and a failed one still leaves the previous state intact.

Retrying inside the leadership term would remove the wait entirely. Both statements are idempotent, so that remains available if a failure turns out to be common enough to warrant it.

## Migration

None.
