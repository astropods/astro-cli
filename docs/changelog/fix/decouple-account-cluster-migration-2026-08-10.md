## Summary

Changing an account's cluster in astro-queen forced immediate migration of every one of its active/failed/pending deployments as a non-optional side effect — there was no way to just repoint an account's cluster (for future deployments) without also migrating everything currently running.

## Design

Split `SetAccountCluster` into two independent admin actions:

- `SetAccountCluster` now only updates `accounts.cluster_id`. Existing deployments keep routing to whatever cluster they're already on.
- A new `MigrateAccountDeployments(account_id)` RPC does what the old bundled logic did — enqueues migration jobs for deployments not yet on the account's current cluster — but is callable independently, anytime, not just as a side effect of a cluster change. It reuses the existing `clusterplacement.ListDeploymentsNeedingMigration` and `enqueueAccountClusterMigrations` helpers unchanged.

In astro-queen's account detail page, `PlacementCard` now shows two separate buttons: **Save** (repoints the cluster) and **Migrate deployments** (always available, enqueues migration for whatever's currently mismatched). No new preview/status endpoint was needed — the existing Deployments page's placement-mismatch badge and per-deployment "Redeploy" (which already syncs routing) already cover ongoing visibility and per-deployment remediation.

Note: `admin.proto`'s generated Go bindings (`admin.pb.go`, `admin_grpc.pb.go`) are hand-maintained in this repo — `buf generate` currently has no `buf.gen.yaml` to run against — so the new `MigrateAccountDeployments` RPC/messages were added by hand, mirroring the existing generated-code shape.

## Migration

None required. `SetAccountClusterResponse`'s `migrations_enqueued`/`deployment_ids` fields are reserved (not reused) in the proto; only astro-queen ever called this RPC.
