# deployment_id is the only redeploy router

## Summary

A deployment row could land in production with `id` and `namespace` referring to two different deployments — e.g. `id = m61-o2n-4t5`, `namespace = astro-qybcsqt8g-0`. Compacting the id (`m61o2n4t5`) does not match the id encoded in the namespace (`qybcsqt8g`), so the row is internally inconsistent: K8s resources live under one identity while the database row carries another.

The cause was redeploy fallback logic in `prepareDeployment`, layered on top of a writer that auto-superseded matching rows and an index that only enforced uniqueness on `'active'`. When a new deploy arrived without a `Target.DeploymentID`, the handler tried to detect "this is really a redeploy" by matching `display_name` or `agent_name` against any active deployment, then minted a fresh id while reusing the matched row's namespace. Even after pulling the handler check, two writer-level twins — a supersede block in `SaveDeploymentPending` and a partial unique index that ignored `'pending'`/`'provisioning'` — kept the same failure mode reachable.

## Design

This change collapses identity to a single rule: **`Target.DeploymentID` is the only signal that a request is a redeploy.** Without it, every deploy is a brand-new row with a fresh id and a namespace derived from that id — full stop. `display_name` is preserved as a per-account uniqueness gate, but it is never used to route a deploy to an existing row.

Three layers move in lockstep so the architectural property actually holds:

**Handler** — the `display_name` / `agent_name` redeploy fallbacks are deleted from `prepareDeployment`. The pre-flight `GetActiveDeploymentByDisplayName` SELECT is gone too: enforcement now lives in the database, and the handler maps the resulting unique-violation to `409 Conflict`.

**Writer** — `SaveDeploymentPending` no longer pre-undeployments any row. The block that ran `UPDATE deployments SET status='undeployed' WHERE display_name=$2 AND status='active'` before the INSERT was the writer-level twin of the handler fallback; with the handler fallback gone it would silently undeploy collisions instead of rejecting them. Removing it lets the partial unique index do its job; a 23505 from the INSERT becomes `ErrDuplicateDisplayName`.

**Schema** — the partial unique index is broadened from `WHERE status = 'active'` to `WHERE status <> 'undeployed'`, covering `pending`/`provisioning`/`scaled_down`/`stopped`/`failed`/`undeploying`. Without this, two concurrent new deploys for the same display name would both insert as `pending` and only collide later when both transitioned to `active`. The index is renamed `idx_deployments_live_display_name` to reflect the broader coverage.

```sql
CREATE UNIQUE INDEX idx_deployments_live_display_name
  ON public.deployments(account_id, display_name)
  WHERE status <> 'undeployed' AND display_name <> '';
```

```go
// prepareDeployment, after this change:
if submittedSpec.Target.DeploymentID != "" {
    // in-place update via UpdateDeploymentPending — id and namespace
    // come from the same row, never drift
}
if k8sNamespace == "" {
    deploymentID = deployid.New()
    k8sNamespace = deploymentNamespace(deploymentID) // = f(id)
}
```

By construction, a fresh deploy now satisfies `namespace == "astro-" + Compact(id) + "-0"`, the in-place-update path keeps both fields untouched, and the database refuses any second live row with a colliding display name. There is no remaining handler or writer path that can emit a row whose id and namespace come from different sources.

## Migration

Schema change is a single index swap — drop `idx_deployments_active_display_name`, create `idx_deployments_live_display_name`. Atlas applies it idempotently on next deploy. Run a pre-flight check for existing collisions in the broader predicate (none expected, since the prior index covered only `active`):

```sql
SELECT account_id, display_name, count(*)
FROM deployments
WHERE status <> 'undeployed' AND display_name <> ''
GROUP BY account_id, display_name
HAVING count(*) > 1;
```

User-visible behavior change: submitting a deploy with the same `display_name` as a live deployment without an explicit `Target.DeploymentID` now returns `409 Conflict` instead of being silently treated as a redeploy. Clients that relied on the old display-name match must pass `Target.DeploymentID` explicitly. The CLI and web UI already do this through `mergeDeploymentPrefill`.

Existing rows with mismatched id/namespace are not rewritten by this change. They remain functional; the invariant only holds for rows created after this PR. A separate one-shot backfill can reconcile prior rows if needed.
