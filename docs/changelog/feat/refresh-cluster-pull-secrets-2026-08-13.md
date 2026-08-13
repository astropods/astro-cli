## Summary

Renaming a cluster's id in config orphans the registry pull credential already baked into every namespace's pull Secret there. An additional cluster's old row, and its still-valid credential, survives only as long as something references it by id via `ON DELETE RESTRICT`. The default cluster has no such protection, since nothing references it by id at all, so renaming it immediately breaks image pulls for everything already running there.

Separately, `buildRegistryConfig` synced cluster config before checking whether the configured default cluster id actually existed in the freshly loaded entries. A `DEFAULT_CLUSTER_ID` drift against the mounted config mutated cluster rows before the function failed, instead of failing before touching the database.

## Design

`buildRegistryConfig` now resolves and validates the default cluster entry before calling `clusterconfig.Sync`, so a default-cluster mismatch fails fast without mutating any rows.

A new admin RPC, `RefreshClusterPullSecrets`, lets an operator explicitly re-push a cluster's current pull credential to every namespace already deployed there, for use right after a rename. It takes the cluster's row id; astro-queen always sends the real id, including for the default cluster. The handler normalizes that real id and the empty-string convention to the same "default" case internally, since `deployments.cluster_id` is `NULL` for default-routed deployments regardless of which form is passed — the caller doesn't need to know that distinction. `deploymentstore.ListActiveNamespacesForCluster` and `k8s.RefreshRegistryPullSecret` do the actual namespace lookup and Secret push, reusing the same write path a deploy's `Apply` already uses.

This is deliberately a manual, operator-triggered action rather than an automatic one. There's no reliable signal that distinguishes "this cluster id is new because of a rename" from "this cluster id is new because it's the very first boot" — an existence check against the database can't tell the two apart. So detection was dropped in favor of an explicit "Refresh pull secrets" button on astro-queen's cluster page, next to the existing health-check and deregister actions.

`admin.proto`'s generated Go bindings are hand-maintained in this repo (no `buf.gen.yaml` to run `buf generate` against), so the new RPC and its request/response messages were added by hand to `admin.pb.go`/`admin_grpc.pb.go`, mirroring the existing generated-code shape.

## Migration

None required. This adds a new opt-in admin action; no existing behavior changes.
