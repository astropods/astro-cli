# cluster_id on deploy spec

## Summary

Adds an optional `target.cluster_id` field to the deployment spec. Absent (the common case) means "route to the primary cluster" — the same default behavior as before. Present pins the deployment to a specific *additional* cluster registered in `public.clusters`; the value is validated against the table at deploy time and persisted to `deployments.cluster_id`.

This is a write-only PR for `cluster_id`. No worker or handler consumer reads the column yet — those follow in subsequent PRs. Behavior for every existing deploy is unchanged: no spec submits `cluster_id`, so every row stays `NULL` and every deployment continues to route to the primary at read time.

## Design

### Field placement

`cluster_id` lives on the existing `target` block of the deployment spec, alongside `runtime`, `account`, `display_name`, and `deployment_id`:

```yaml
target:
  runtime: kubernetes
  cluster_id: eu-west-1-managed   # optional; omit to route to primary
```

That block is already the "where this deployment runs" surface of the spec, so adding a target-cluster field there matches reader intuition and avoids inventing a new request envelope. The field is `omitempty` in both JSON and YAML tags, so existing CLI/spec consumers that don't know about it stay valid.

### Validation

The deploy handler short-circuits on a non-empty `target.cluster_id`:

- Look up via `clusterstore.Get(ctx, id)`.
- `ErrNotFound` → `400 {"error": "unknown cluster_id", "cluster_id": "<id>"}`.
- Row exists but `enabled = false` → `400 {"error": "cluster is disabled", "cluster_id": "<id>"}`.
- Otherwise the deploy proceeds normally and the id flows through `SaveDeploymentParams.ClusterID`.

The validation runs *before* `prepareDeployment`, so a bad `cluster_id` rejects fast with zero DB writes against `deployments`, `agent_versions`, etc.

### Persistence

`SaveDeploymentParams` gains a `ClusterID string` field, mirroring how `SourceAccountID` and `KMSKeyARN` already model nullable text columns. The INSERT in `SaveDeploymentPending` and the UPDATE in `UpdateDeploymentPending` pass `nilIfEmpty(p.ClusterID)` — empty string becomes NULL, non-empty is stored verbatim. The FK on `deployments.cluster_id → clusters(id)` (added in PR 1) is what makes the validation step necessary; without it a deploy with a bogus id would either fail at INSERT time with a less-friendly error, or (with a NULL FK column) silently land with no real row to point at.

Redeploys behave the same as new deploys: whatever `target.cluster_id` the new spec carries overwrites the previous value. An empty `target.cluster_id` on a redeploy clears the column back to NULL.

## Migration

None. Every existing deployment row keeps `cluster_id = NULL`. The new field is optional and back-compat for any spec that omits it.
