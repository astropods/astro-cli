# deploy-stability-fixes — env vars from the DB, on the record endpoint

## Summary

Two problems with how the runtime endpoint (`GET /api/v1/deployments/:id/runtime`) handled environment variables:

1. **Wrong source.** It resolved env by reading per-pod `EnvFrom` references back out of Kubernetes — issuing a `Secret`/`ConfigMap` GET for every reference, every poll, every viewer. Sustained K8s API load that scales with viewers × workloads × envFrom refs.
2. **Wrong endpoint.** Even with the source fixed to the DB, env was still being attached to the runtime view. But env is *apply-time intent*, not live cluster state — exactly the kind of data the record/runtime split (documented at `deploy.go:1187`) says belongs on the record endpoint.

This change does both: env reads come from `deployment_build_env`, and they're exposed on `GET /api/v1/deployments/:id` (record), not on the runtime endpoint.

## Design

**Single source of truth: the DB.** A new `deploymentstore.LoadDecryptedBuildEnv` reads `deployment_build_env`, decrypts non-secrets via the deployment's envelope key (KMS-off mode passes through plaintext), redacts secrets to `••••••••`, and returns env grouped by role.

**Env lives on the record endpoint.** `WorkloadSpec` gains an `Env map[string][]EnvVar` keyed by role:

```go
type WorkloadSpec struct {
  ...existing fields...
  Env map[string][]EnvVar `json:"env,omitempty"`  // keys: "agent" | "messaging" | "collector" | "knowledge:<n>" | "ingestion:<n>"
}
```

A workload's *component* determines which roles apply to it (`rolesForComponent`): `"agent"` workloads carry both `"agent"` and `"messaging"` so a client can render env for both the agent container and its messaging sidecar off one workload entry. All other components carry their single role.

**Runtime carries no env.** `ContainerStatus` no longer has an `Env` field — that field is gone from `WorkloadRuntime.Containers` entirely. `buildContainerStatuses` was already trimmed to status-only (state, ready, restart_count, reason, message).

**Client zips env per-container.** `roleFor(component, containerName)` (5 lines in `lib/env-utils.ts`) maps a container to its role; `PodDetailPanel` looks up `workload.env[role]` to render each container's env. Container names stay where they belong: on the runtime side, alongside their live state.

**API change.** Only `EnvVar.From` is dropped — the DB stores resolved values, not K8s refs, so the field carried no information once env moved off the live pod. `Name`, `Value`, `Source`, `IsSecret` are retained. `ContainerStatus.Env` is removed. `WorkloadSpec.Env` is new. Existing API tests confirmed by the AgentDeployments suite.

**Convention captured.** Added a "Kubernetes API Usage" section to the top-level `CLAUDE.md`: K8s reads are for cluster state only (replicas, phase, readiness, conditions). Anything the server already wrote to the DB at apply time — env, spec, URLs, intent — must come from the DB. The runtime/record split exists precisely to keep this clean.

## Migration

No client-side migration. Operators get two observable wins:

- Per-poll Secret/ConfigMap GET-storm on the runtime endpoint is gone — `O(viewers × workloads × envFrom)` K8s reads collapse to one DB query on the record endpoint.
- During a rolling update, the env panel reflects the deployed spec immediately on apply instead of lagging until the new pod reaches `Running`. (Previously, with the new pod `Pending` and the old pod `Running`, the runtime endpoint returned the old revision's env.)

Pre-cutover deployments without rows in `deployment_build_env` render with an empty env map rather than the prior K8s-derived list. Re-apply (or a redeploy from the UI) populates the table.
