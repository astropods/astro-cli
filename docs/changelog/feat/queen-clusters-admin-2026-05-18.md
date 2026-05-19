# astro-queen Clusters admin UI (M0 Phase 1, PR 7)

## Summary

Operators can manage additional Kubernetes clusters from the queen web admin UI instead of grpcurl. The page lists the primary cluster (env-defined) plus registered additional clusters, with register, enable, disable, and deregister actions.

## Design

- **HTTP proxy**: Five routes under `/api/admin/clusters` forward to PR 6 admin gRPC (`ListClusters`, `RegisterCluster`, `EnableCluster`, `DisableCluster`, `DeregisterCluster`) in a dedicated `cluster_handlers.go`, separate from namespace workload `GET /api/admin/cluster-status`.
- **React**: `ClustersPage` uses TanStack Query hooks (`useClusters`, mutation hooks) with query-key invalidation on writes. Primary rows are read-only; additional rows support lifecycle actions and destructive deregister via `AlertDialog`.
- **Navigation**: Admin sidebar link at `/admin/clusters`.

## Migration

Requires astro-server with cluster admin gRPC (PR 6). Queen operators need no config changes beyond an updated queen binary or local build. No astro-client or deploy-form changes in this PR.
