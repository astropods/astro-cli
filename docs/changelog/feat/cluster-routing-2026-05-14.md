## Summary

River workers and the deploy path resolve Kubernetes clients per deployment `cluster_id`: additional clusters load through `k8s.Registry.Get` (backed by `clusterstore` and an in-process cache). Primary routing is unchanged when `cluster_id` is null or empty.

## Design

- **Registry** — `NewRegistry` takes `*clusterstore.Store`; `Get(ctx, id)` returns cached `ClusterClient` or builds an EKS client from the row; `ErrClusterNotFound` / `ErrClusterDisabled` surface to callers without silent fallback to primary.
- **Deployment reads** — Full-row scans include `cluster_id`; `EffectiveClusterID()` yields the routing key for jobs and workers.
- **Deployer** — Holds `*k8s.Registry` and resolves the client per deployment for apply/teardown (no shared mutable client per request).
- **River** — `riverqueue.Config` carries `K8sRegistry`. Deploy, undeploy, wakeup, and GitHub build job args include optional `cluster_id` (JSON); enqueue paths copy from the deployment row or save params. Reconcile and knowledge workers use the registry; knowledge K8s touchpoints stay **primary-only** until knowledge rows carry cluster metadata.
- **API wiring** — `main` passes one `clusterstore` into `NewRegistry` and into route setup for spec validation; workers receive the same registry shape as API mode when both run.

## Migration

No operator action for single-cluster installs. Multi-cluster routing requires registered enabled clusters and non-null `deployments.cluster_id` (already written by prior deploy-spec work); invalid or disabled ids fail the async job with the same retry/dead-letter behavior as today.
