# Admin additional-cluster CRUD (M0 Phase 1, PR 6)

## Summary

Operators can register, enable, disable, and deregister additional Kubernetes clusters at runtime through the admin gRPC API. The primary cluster remains env-var-defined and read-only in admin responses. Registry client cache is invalidated on lifecycle changes so disabled or updated rows are not served stale clients.

## Design

- **`k8s.Registry`**: `List` synthesizes a read-only `primary` entry plus `clusterstore.List` rows; `Refresh` evicts cached additional clients after mutations.
- **Admin gRPC**: `RegisterCluster`, `EnableCluster`, `DisableCluster`, `DeregisterCluster`, `ListClusters` delegate to `clusterstore` and call `Refresh` after writes. Mutations targeting `primary` return `InvalidArgument`.
- **Health**: Each `RegisteredCluster` in list/mutation responses includes `healthy` / `health_error` from `CheckHealth` on the primary or enabled additional client; disabled rows report `cluster disabled`.
- **Wiring**: `admingrpc.Server` receives `clusterstore` and `*k8s.Registry` from `main` when the API registry boots successfully.

## Migration

No action required. Existing single-cluster deployments are unchanged until an operator registers additional clusters via admin tooling (astro-queen in PR 7).
