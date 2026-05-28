# Per-cluster ingress, ALB, cert, and knowledge configuration

## Summary

Until now astro-server pulled ingress hostnames, ACM certificate ARNs, ALB
group names, and the public knowledge-store domain from a single set of
process-wide env vars (`INGRESS_DOMAIN`, `ACM_CERTIFICATE_ARN`,
`ALB_GROUP_NAME`, `INGESTION_*`, `KNOWLEDGE_DOMAIN`). Those values reflect
the cluster astro-server itself is bootstrapped against — the **primary**
cluster — but were also used for deployments routed to any **additional**
cluster registered through the admin gRPC. The result: a deploy targeting a
non-primary cluster ended up with the primary cluster's ALB group, primary
cluster's certificate ARN, and primary cluster's ingress domain. ALB
provisioning silently fell back to the wrong account / region, certificates
didn't match the actual host, and knowledge stores got public hostnames in
the wrong zone.

Each registered cluster now owns its ingress configuration. The primary
cluster keeps reading env vars. Additional clusters must declare a complete
config when they are registered and have no fallback to env defaults.

## Design

### Schema

`public.clusters` gains seven NOT NULL columns (default `''`):

```
agent_ingress_domain
agent_acm_certificate_arn
agent_alb_group_name
ingestion_ingress_domain
ingestion_acm_certificate_arn
ingestion_alb_group_name
knowledge_domain
```

The `agent_*` prefix mirrors the existing `ingestion_*` convention — both
groups describe one ALB each, named for the workload it fronts.

The defaults make this a forward-compatible diff: existing additional-cluster
rows acquire empty strings on apply, which is exactly the state a freshly
created row was in before. The new validation rules below then surface those
empties at register / update / deploy time so an operator is forced to fill
them in rather than have the deployer silently use primary-cluster values.

### Resolution

A new package `internal/clustercfg` owns the per-deployment resolution. It is
called by the deployer (when building `ApplierConfig` and when computing
`externalAgentHost`), the deploy submit handler (when persisting normalized
ingress rows), the knowledge create handler (when computing the public
hostname), and the admin gRPC repair / backfill paths.

```go
resolved, err := clustercfg.Resolve(ctx, k8sReg, cfg.Deployment, clusterID)
```

- `clusterID == ""` or `PrimaryClusterID` → returns the env-derived defaults
  from `cfg.Deployment.*`.
- Any other id → reads the cluster row through the registry and returns its
  values verbatim. **Env defaults never apply to non-primary clusters.**
- Unknown clusters and rows missing any required ingress field produce an
  error rather than silently degrading.

### Strict registration

`clusterstore.Register` and `clusterstore.Update` now require every
ingress / cert / knowledge field to be non-empty for additional clusters.
The same validation re-runs at deploy time so legacy rows (NULL on the day
of the schema change) cannot quietly fall through.

### Registry entry cache

`k8s.Registry` now caches `ClusterEntry` values alongside its existing
`ClusterClient` cache. The deploy submit path needs the entry twice (once
for validation, once for ingress resolution); without caching that would
be two DB hits per deploy. `Refresh(id)` evicts both caches; the admin RPCs
already call it after every mutation.

### Admin gRPC surface

`RegisteredCluster`, `RegisterClusterRequest`, and `UpdateClusterRequest`
gain the seven new fields. `astro-queen` and any other admin client must
supply them when registering a cluster — empty values are rejected.

### astro-queen UI

The clusters page register-form and edit-dialog now expose the seven
per-cluster ingress fields under a labelled "Ingress / ALB / cert / knowledge"
fieldset. The Register and Save buttons stay disabled until every field is
filled, mirroring the strict server-side validation.

## Migration

For an operator running additional clusters today:

1. Apply the new schema columns (no backfill — they default to `''`).
2. For each registered cluster, call `UpdateCluster` (or the equivalent in
   `astro-queen`) with the cluster's real ingress domain, ACM ARN, ALB group
   name, ingestion equivalents, and knowledge domain. Until this is done,
   deploys targeting that cluster will fail fast with a clear "cluster X is
   missing required ingress field Y" error.
3. The primary cluster needs nothing — it continues to read env vars.

No behavior change is observable for deployments that target the primary
cluster.
