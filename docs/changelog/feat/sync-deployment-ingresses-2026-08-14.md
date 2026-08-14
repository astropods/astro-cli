## Summary

The messaging proxy (chat, files, and generic messaging endpoints) had two ways
to reach a deployment's messaging sidecar: the tenant-router Envoy path, and a
fallback through the Kubernetes API server's `services/proxy` subresource.
The fallback predates the tenant-router migration and is no longer needed —
every cluster now carries a `TenantRouterInternalURL`. It's removed.

Removing the fallback exposes a real gap: an Ingress only picks up routing
changes (host rules, the tenant-router `IngressClassName`, the internal
messaging host) when something else in the same deploy also changes and
triggers a full apply. A deployment nobody has redeployed since the
tenant-router migration landed can be running fine while its Ingress still
reflects the old shape, so messaging traffic for it has nowhere to go once the
fallback is gone.

An initial version of this fix added one admin action that blindly re-applied
every active deployment's Ingress in bulk. That doesn't generalize: the next
migration would need its own bespoke button, with no record of which
deployments were ever checked or what was wrong with them. This lands a
declarative alternative instead.

## Design

**`internal/deployeval`** is a new package for deployment-configuration-drift
checks. An `Evaluator` names one check and its fix:

```go
type Evaluator interface {
	ID() string
	Name() string
	Description() string
	Check(ctx context.Context, dep *deploymentstore.Deployment) (CheckResult, error)
	Fix(ctx context.Context, dep *deploymentstore.Deployment) error
}
```

`Store` persists the last result per `(deployment_id, evaluator_id)` in a new
`deployment_evaluator_state` table (`status`: `ok` | `drifted` | `fix_failed`,
plus `detail`, `checked_at`, `fixed_at`), so astro-queen can list what's
currently drifted without re-checking the whole fleet on every page load.

Evaluators self-register with a `Factory` from an `init()` in their own file:

```go
func init() { Register(func(d Deps) Evaluator { return NewFoo(d.Deployer) }) }
```

`BuildAll(deps)` constructs every registered evaluator. Adding the next
migration's evaluator is "add a file" — no other file changes, since
admingrpc and astro-queen already list, sweep, and fix whatever `BuildAll`
returns. `Deps` bundles the dependencies a factory may need; a future
evaluator needing something new adds a field there rather than a new
registration signature.

The first evaluator, `tenant-router-ingress`, wraps the Ingress logic this
change actually needed: `Applier.desiredIngresses` (previously inline in
`ApplyDeploymentSpec`) is now shared by `Applier.SyncIngresses` (apply) and
the new `Applier.CheckIngressDrift` (read-only compare against the live
object's class and host/backend rules). `Deployer.CheckIngressDrift` and
`Deployer.SyncIngresses` wrap these for one deployment without secret
rehydration or Langfuse/AI Gateway provisioning — no side effects beyond the
Ingress itself.

Four admin RPCs back the astro-queen panel: `ListEvaluators` (catalog +
per-evaluator ok/drifted/fix_failed counts), `RunEvaluatorSweep` (checks every
active deployment for one evaluator, synchronously — each check is a cheap
K8s Get), `ListEvaluatorDrift` (drifted rows for one evaluator, with
deployment/account identity), and `FixDeploymentDrift` (runs Fix for one
deployment, re-checks, and persists the real post-fix status rather than
trusting that Fix returning no error means the drift resolved).

astro-queen's Deployments page gets an "Evaluators" panel below the table:
each evaluator shows its counts and a "Run check" button; expanding one lists
its drifted deployments with a per-row "Fix" button.

## Migration

None required for the schema (declarative — `sql/astro-server/schema.sql`
picks up `deployment_evaluator_state` on the next `atlas schema apply`). The
kube-apiserver-proxy fallback removal only affects clusters without a
`TenantRouterInternalURL` configured — every current environment has one set.
Existing deployments whose Ingress predates the tenant-router migration show
up as drifted the first time an operator runs the `tenant-router-ingress`
evaluator from astro-queen, and can be fixed individually from there.
