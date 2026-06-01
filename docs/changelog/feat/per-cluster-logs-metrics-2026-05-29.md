# Per-cluster logs and workload metrics queries

## Summary

Deployments on additional clusters (e.g. EU) showed empty pod metrics and missing logs in the UI because astro-server always scoped Prometheus queries to the **primary** EKS cluster name from `EKS_CLUSTER_NAME`, while Alloy labels series with each cluster's `eks_cluster_name`. Loki queries also omitted the `cluster` stream label.

## Design

- **`k8s.Registry` helpers** — `PrometheusClusterFilter(ctx, deploymentClusterID)` and `LokiClusterName(ctx, deploymentClusterID)` resolve the target cluster via `GetEntry` (empty id → primary bootstrap name; non-empty → `clusters.eks_cluster_name`).
- **Handlers** — `resolveDeploymentContext`, deployment logs/stream, and admin `GetPodLogs` use the deployment's `EffectiveClusterID()` instead of the process-global Prometheus client cluster.
- **Loki** — `QueryParams.Cluster` adds `cluster="<eks_name>"` to LogQL selectors when set.

Shared Loki/Prometheus endpoints are unchanged; only query scoping is per deployment routing.

## Migration

Deploy astro-server after EU (or any additional) cluster rows have correct `eks_cluster_name` matching Alloy's `cluster_name` template variable. Verify Alloy on the additional cluster is shipping to the shared observability stack (VPC peering or platform PrivateLink URLs).
