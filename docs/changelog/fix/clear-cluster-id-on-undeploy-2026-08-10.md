# Clear cluster_id when a deployment is undeployed

## Summary

`deployments.cluster_id` and `accounts.cluster_id` both carry `ON DELETE RESTRICT` foreign keys into `clusters`. Deregistering a cluster failed with a generic "cluster has active deployments" error whenever either FK was still referenced — including from deployments that had already been torn down. `updateStatusTx` set `status = 'undeployed'` but never cleared `cluster_id`, so a fully undeployed row kept pointing at a cluster it no longer runs on, blocking deregistration even when the admin UI showed zero active deployments for it.

## Design

**Clear `cluster_id` at the same time as the status transition.** `updateStatusTx` (`apps/astro-server/internal/deploymentstore/store.go`) now nulls `cluster_id` in the same `UPDATE` that sets `status = 'undeployed'`. Nothing downstream depends on the old value surviving: a later redeploy sets a fresh `cluster_id` from the submitted spec (`UpdateDeploymentPending`), and no billing or audit path reads `cluster_id` gated on undeployed status.

**Disambiguate which FK actually blocked deletion.** `clusterstore.Store.Deregister` used to map any `23503` (foreign_key_violation) to one generic `ErrInUse`. It now reads the violated constraint name off the Postgres error and returns `ErrInUseByAccounts` or `ErrInUseByDeployments` accordingly, falling back to `ErrInUse` only if the constraint name is unrecognized.

**Surface what's actually blocking.** A new `GetClusterBlockers` RPC (`clusterstore.Store.Blockers`, wired through astro-queen's `GET /api/admin/clusters/{id}/blockers`) lists up to 25 accounts and 25 deployments still referencing a cluster, plus total counts. astro-queen's cluster detail panel fetches this automatically the moment a deregister attempt fails, so the admin sees exactly which rows to clear instead of a bare error string.

**Placement-mismatch fallout.** Clearing `cluster_id` on undeploy meant `EffectiveClusterID()` now returns empty for undeployed rows, which would otherwise show a spurious "placement mismatch" badge against whatever cluster the owning account is pinned to. `populateAdminDeploymentPlacement` now skips the mismatch check when the deployment's status is `undeployed`.

**Self-heal stale rows at deregister time instead of a manual backfill.** Rows undeployed before this fix still carry a stale `cluster_id` and would otherwise need a one-off `UPDATE`. `Deregister` now retries once: on `ErrInUseByDeployments` it runs `UPDATE deployments SET cluster_id = NULL WHERE cluster_id = $1 AND status = 'undeployed'`, then attempts the delete again. If a genuinely active deployment is still attached, the retry fails the same way and the caller sees the same error — the self-heal is a no-op in that case, not a mask. Accounts are deliberately excluded from this: an account's `cluster_id` is a live routing decision, not stale history, so `ErrInUseByAccounts` still requires an explicit fix (repoint the account, or move/undeploy its deployments).

## Migration

No schema change — `cluster_id` was already nullable on both tables. No manual backfill needed: the first `Deregister` (or admin "Deregister" click) attempt against a cluster with only stale undeployed rows now clears them automatically and succeeds.
