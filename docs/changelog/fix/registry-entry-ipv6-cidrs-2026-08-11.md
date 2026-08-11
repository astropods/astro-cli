# Fix pod_subnet_ipv6_cidrs dropped by k8s.Registry's cluster-row mapping

## Summary

#1931 added `pod_subnet_ipv6_cidrs` to `clusterstore.Cluster` and to `admingrpc.rowToEntry` (the mapping `RegisterCluster`/`UpdateCluster`/`GetEntry`-via-admingrpc paths use), but missed that `k8s.Registry` has two of its own, separate inline `ClusterEntry{...}` conversions — in `GetEntry` and `List` — that duplicate the same row-to-entry mapping instead of calling a shared helper. Both were missed, so the field was silently dropped.

`GetEntry` is what `clustercfg.Resolve` actually calls on every deploy, so this wasn't just a display bug: updating a cluster's `pod_subnet_ipv6_cidrs` in the database (directly, or via `UpdateCluster`) had no effect — `Applier.applyNetworkPolicies` always saw an empty value and never added the `::/0` peer. Caught while verifying the pm-eu IPv6 NetworkPolicy fix end-to-end: `pod_subnet_ipv6_cidrs` was correctly set in `public.clusters`, but the `ListClusters` admin API (and, more importantly, an actual deploy) still showed it empty.

## Design

Fixing the immediate symptom (add the missing field copy in two places) would leave the actual hazard in place: three independent hand-written struct literals doing the same `clusterstore.Cluster` → `k8s.ClusterEntry` mapping, with nothing forcing them to stay in sync. Adding a field to either struct means remembering to update all three, and a `grep <new-field-name>` won't catch the ones you missed — unmigrated code by definition doesn't mention the new field yet.

Instead, all three conversion sites now share one function, `k8s.ClusterEntryFromRow(*clusterstore.Cluster) ClusterEntry`:
- `Registry.GetEntry` and `Registry.List` (the two that were dropping the field) call it directly.
- `admingrpc.rowToEntry` becomes a one-line delegate to it, so its three callers (`RegisterCluster`, `UpdateCluster` via `loadAdditionalEntry`, `loadAdditionalEntry` itself) are unaffected.

No behavior change for the field's semantics — same optional, default-empty field from #1931 — just one conversion site instead of three drifting copies.

Added regression coverage: `TestRegistry_GetEntry_IncludesPullCredential` and `TestRegistry_List_IncludesPrimaryAndRows` now seed a non-empty `pod_subnet_ipv6_cidrs` in their mock rows and assert it survives the round trip (previously both used `""`, which couldn't distinguish "field flows through" from "field is dropped"). Both tests fail against the pre-fix code.

## Migration

No action required. Once this deploys, `pm-eu`'s already-updated `clusters` row will actually take effect on its next deploy/reconcile — no further DB changes needed.
