# Deployment Spec Normalization — Phase 2 & 3

## Summary

Builds on the Phase 1 normalized tables to switch read paths away from JSON parsing and add a drift detection reconciler. The deployment JSON blob is still dual-written for backward compat, but consumers now read structured data from the normalized tables when available.

## Design

### Phase 2 — Read from Normalized Tables

**OpenMeter heartbeat** now queries `deployment_workloads` via a direct JOIN on `deployments` instead of parsing `deployment_spec_json` into `AstroDeploymentSpec`. Falls back to JSON for old deployments without normalized data.

```sql
SELECT d.account_id, d.agent_name, d.namespace,
       w.component_kind, w.component_key, w.replicas, w.cpu_request, w.memory_request
FROM deployments d
JOIN deployment_workloads w ON w.deployment_id = d.id
WHERE d.status = 'active'
```

**Admin gRPC `ListDeployments`** populates the `Components` field (previously always empty) from `deployment_workloads`, giving the TUI visibility into what each deployment contains (e.g. `agent`, `model/gpt4`, `knowledge/redis`).

New store query methods: `GetWorkloadSummaries`, `GetActiveDeploymentWorkloads`, `GetServices`, `GetIngresses`.

### Phase 3 — Drift Checker (Report-Only)

New `internal/driftcheck` package runs a periodic loop (every 10 minutes, in worker mode) that compares desired state from the normalized DB tables against actual K8s cluster state:

| Resource | Checks |
|----------|--------|
| Deployment | Existence, replica count, container image |
| StatefulSet | Existence, replica count, container image |
| CronJob | Existence, schedule match |
| Service | Existence by name |
| Ingress | Hostname presence |

Detected drift is logged as structured warnings — no auto-remediation. Skips pre-normalization deployments (no workload rows). Guarded by `k8sClient != nil` (disabled when K8s is unavailable).

Drift types: `missing`, `replicas`, `image`, `schedule`.

## Migration

No action required. The drift checker is purely observational and starts automatically in worker mode.
