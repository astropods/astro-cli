# Multi-region cluster support — registry

## Summary

Replaces the legacy process-wide `k8s.ClusterClient` singleton with a `k8s.Registry` that owns the primary `ClusterClient`. The primary is built from the same env vars / kubeconfig that defined the legacy singleton (`EKS_CLUSTER_NAME` / `K8S_MASTER_URL` / `AWS_REGION` in EKS mode, `KUBECONFIG` / `KUBE_CONTEXT` in local mode). Every handler and worker now receives `registry.Default()` exactly where the singleton was previously plumbed — observable behavior is unchanged.

The PR also introduces the architectural separation that the rest of the multi-region series builds on: the **primary cluster** (env-var-defined, one per process, owned by the deployment artifact) is distinct from **additional clusters** (rows in `public.clusters`, registered at runtime via a future admin API). No additional clusters are added or read in this PR.

## Design

### `k8s.Registry`

A thin struct holding one ClusterClient field:

```go
type Registry struct {
    primary ClusterClient
}

func NewRegistry(ctx, cfg RegistryConfig, log) (*Registry, error) { ... }
func (r *Registry) Default() ClusterClient { return r.primary }
```

`NewRegistry` builds the primary via the existing `NewClusterClient` factory and returns an error if construction fails — astro-server fails to boot rather than running without a routable cluster.

### Why no backfill, no `DEFAULT_CLUSTER_ID`

An earlier draft materialised the primary as a row in `clusters` on first boot, with a `DEFAULT_CLUSTER_ID` env var pointing the registry at it. That created two sources of truth for one value (env vars + table) and required boot-time SQL writes plus a kubeconfig parse on first boot in local mode.

The current design treats env vars as the authority for the primary and the `clusters` table as the authority for additional clusters. The table stays empty in this PR; rows are added later when the admin API lands. Side effects:

- No `DEFAULT_CLUSTER_ID` env var, no resolution rules, no config field, no `sanitizeClusterID` helper.
- No backfill SQL, no `ON CONFLICT DO NOTHING` race handling, no `clusterstore.Register` call from `NewRegistry`.
- No kubeconfig parse during boot — `LocalClient` handles its own kubeconfig as before; the registry never re-parses it for backfill purposes.

One structural consequence: the primary is not visible as a row to admin tooling. `ListClusters` (in a future PR) will need to synthesize a primary entry alongside DB rows. This is a one-place special case in exchange for never having to keep two stores of "what's the primary cluster" in sync. The primary's lifecycle is the astro-server deployment's lifecycle — to change it, redeploy with different env vars.

### `deployments.cluster_id`

Unchanged from PR 1. The semantics this PR commits to:

- `NULL` → primary cluster (the one in env vars).
- non-null → an additional cluster (row in `clusters`).
- `ON DELETE RESTRICT` on the FK still applies, protecting additional clusters from being deregistered while deployments reference them.

PR 2 does not read or write `cluster_id` from any handler — it stays NULL on new deploys and continues to mean "primary."

### `main.go` wiring

Both API-mode and worker-mode init paths construct a `k8s.Registry` from the same `cfg.Deployment` fields that previously fed `k8s.NewClusterClient` directly, then pass `registry.Default()` to every consumer (handlers, workers, admin gRPC server). Function signatures downstream are unchanged.

### What this PR consumes vs reserves

`Default()` is the only registry method any caller in this PR invokes. `Get(ctx, id)` / `List() / Refresh(ctx, id)` and their supporting error types are deliberately **not** introduced in this PR — they'll land alongside their first callers in subsequent PRs, so the registry surface grows with proven demand rather than as speculative scaffolding.

The `clusterstore` package (from PR 1) similarly retains its full CRUD surface unused for now; the admin API in a later PR is its first consumer.

## Migration

No data migration. No env var changes. No schema changes beyond PR 1.

- The first `astro-server` boot after this change builds a primary `ClusterClient` from the existing env vars, identical to the legacy singleton. `clusters` table stays empty.
- Existing deployments keep `cluster_id = NULL` and are resolved to the primary at read time once per-deployment routing lands.
- `EKS_CLUSTER_NAME`, `K8S_MASTER_URL`, `AWS_REGION`, `K8S_CLIENT_MODE`, `KUBECONFIG`, `KUBE_CONTEXT` continue to work exactly as before — they are now read in one place (`config.Load`) and consumed by the registry.

Rolling back: revert this PR. The legacy singleton path is reachable from history; behavior was identical to `registry.Default()` so nothing else changes.
