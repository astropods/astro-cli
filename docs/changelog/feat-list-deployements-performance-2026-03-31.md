# Deployment list: parallel K8s fetches and reduced per-deployment calls (steps 2–3)

## Summary

The deployment list endpoint (`GET /api/v1/deployments`) was making K8s API calls sequentially and fetching far more data than needed. For an account with N deployments the total wall time was `N × k8s latency`, and each deployment triggered 5+ K8s calls (Deployments, StatefulSets, Ingresses, Pods, Jobs, and per-pod ConfigMap/Secret lookups) even though the list view only requires status and replica counts.

## Design

**Parallelized K8s fetches** — the sequential loop over DB deployments in `ListDeployments` is replaced with a `golang.org/x/sync/errgroup` fan-out. Each deployment's K8s enrichment runs in its own goroutine. Results are pre-allocated by index to avoid a mutex. Errors fall back to a DB-only entry and do not cancel other goroutines. Wall time drops from `N × k8s latency` to `1 × k8s latency`.

**Light K8s enrichment for the list path** — a new `listAstroDeploymentsLight` function makes only 2 K8s calls per namespace (Deployments + StatefulSets) instead of the full set. It returns status, replicas, ready, and components — the fields needed to determine deployment health. Ingresses, Pods, Jobs, and the per-pod ConfigMap/Secret lookups are skipped entirely. The full `listAstroDeployments` is unchanged and continues to be used by `GetDeployment` (the detail endpoint).

`enrichDeployment` is parameterised with a `k8sListFn` so both paths share the same namespace existence check and DB field merging logic. `ListDeployments` passes `listAstroDeploymentsLight`; `GetDeployment` passes `listAstroDeployments`.

## Migration

No action required.
