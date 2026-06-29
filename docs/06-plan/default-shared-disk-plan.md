# Default Shared Disk for Every Deployment — Plan

> **Status: implemented.** This document reflects the shipped design.

## Context

Every agent deployment provisioned compute and memory by default, but **disk was optional** — it existed only when the spec set `agent.volume`; otherwise the agent ran as an ephemeral `Deployment` with no persistent storage. The goal: make disk a guaranteed default — **every** agent gets a persistent volume, with no enable/disable choice — and **share that volume between the agent container and the messaging sidecar**, so messaging can later persist conversation history (sqlite) on it.

This covers the **foundation only**: always-provision the disk and mount it into both containers. It does **not** implement the messaging sqlite store (deferred — "layer 3"). After this, enabling sqlite is a localized change in the messaging module plus flipping `STORAGE_TYPE`.

## Key facts

- Workload branch keyed on `agent.volume`: set → **StatefulSet** with a `volumeClaimTemplate` named `data` mounted at `agent.volume`; empty → ephemeral **Deployment**, no PVC.
- Messaging is a **native sidecar** (init container, `restartPolicy: Always`) in the **same pod** as the agent (`deployment.go`, `statefulset.go`) — so a single RWO PVC can be shared by both.
- The deploy path does **not** re-run the template generator — it loads the stored spec JSON verbatim (`deployer.go`) and applies it. So defaulting only at generation would miss redeploys of legacy/stored specs.
- Compute/memory already default via `BuildResourceRequirements` + fallbacks — no change needed.

## Approach: default the volume at the apply choke point + reuse the StatefulSet path

Every deploy — fresh, redeploy, configure — funnels through `ApplyDeploymentSpec`. A normalization step there (`normalizeAgentStorageDefaults`) sets `agent.volume` + `agent.storage` when empty, so **all** deployments route through the existing StatefulSet + PVC machinery. The template generator also seeds the default (for the Configure UI's provisioning echo + cost preview), but the apply-time normalize is the actual guarantor — it covers legacy/stored specs the generator never re-touches. The messaging sidecar then mounts the shared `data` volume under a subPath.

Because the volume is now guaranteed, the agent is **always** a StatefulSet — the stateless-Deployment branch for agents (and the `volume != ""` checks guarding it) are removed.

**Why StatefulSet (not `emptyDir` or a shared named PVC):** persistence must survive pod restart/reschedule/redeploy (the eventual sqlite history). `emptyDir` is ephemeral. A Deployment sharing one ReadWriteOnce PVC breaks past one replica (block storage attaches to a single node) and forces us to own PVC lifecycle. StatefulSet's per-pod `volumeClaimTemplate` gives each replica its own RWO disk, stable identity, and K8s-managed PVC lifecycle.

## Changes

### 1. `packages/astro-spec/deployment_spec.go`
`DefaultAgentVolumeMount = "/data"` and `DefaultAgentStorageSize = "5Gi"` (a modest universal baseline; aligns with the deploy UI's smallest storage tier). `DefaultStorageConfig()` (10Gi) stays for explicitly-requested volumes.

### 2. `apps/astro-server/internal/k8s/spec_applier.go` — apply-time default (the guarantor)
`normalizeAgentStorageDefaults(ds)` at the top of `ApplyDeploymentSpec`: if `agent.volume == ""`, set the default mount + storage. Idempotent; explicit values win. `applyAgentWorkload` now always builds a StatefulSet (dead Deployment branch removed); `computeExpectedResourceNames` always expects the agent StatefulSet (so a legacy agent's stale Deployment is torn down as an orphan).

### 3. `apps/astro-server/internal/deployment/template.go` — generation-time default
`GenerateTemplate` seeds `Volume` + `Storage` on the agent block so the Configure UI shows the effective values. Not the guarantor (redeploys bypass it), but keeps the UI honest.

### 4. Messaging sidecar mounts the shared `data` volume
`MessagingDeploymentConfig` gains `VolumeName`/`VolumeMountPath`/`VolumeSubPath`; `buildMessagingContainer` appends the mount. Shared constants `agentDataVolumeName = "data"` and `messagingVolumeSubPath = "messaging"`. The sidecar mounts the `data` volume at `/data` under subPath `messaging` — its own isolated subtree on the same disk. RWO suffices (same pod).

### 5. Frontend (`astro-client`)
Removed the enable/disable "Persistent volume" toggle (`VolumePicker` deleted) and the vestigial `volumeEnabled` plumbing. Storage is always shown in "Advanced sizing" (default tier `5Gi`); the **mount path** is an optional advanced field defaulting to `/data` — preserving the prior ability to set a custom path. The provisioning override is sent when the user customizes mount or size, mount falling back to `/data`.

## Migration

- **Existing agents deployed as `Deployment`** flip to `StatefulSet` on their next redeploy: the stored spec (empty volume) is normalized at apply time, the StatefulSet is created, and orphan cleanup removes the now-unexpected `Deployment`. One-time pod recreate; no user action, no spec changes.
- Specs that already set `agent.volume` are unaffected (explicit value wins).

## PVC lifecycle & cleanup

Two distinct deletion paths, governed by different mechanisms — do not conflate them:

- **Namespace teardown (undeploy)** already deletes PVCs unconditionally. `deleter.go` runs `deletePVCs` (lists and deletes every PVC in the namespace) and then `deleteNamespace` (cascades to any remaining namespaced object). This is **independent of the StatefulSet retention policy** — a deleted namespace always takes its PVCs with it. **Requirement satisfied: namespace delete ⇒ PVC delete.**
- **StatefulSet delete/scale (namespace survives)** is the only case the `PersistentVolumeClaimRetentionPolicy` governs. Because namespace teardown is handled separately, we are free to set `WhenDeleted: Retain` so the disk survives a redeploy that recreates the StatefulSet, **without** risking orphaned PVCs on undeploy.

**Recommendation:** set `WhenDeleted: Retain`, `WhenScaled: Delete` on the agent StatefulSet (matching knowledge stores) so persistent data survives StatefulSet recreation but scaled-down replicas don't leak disks. Undeploy still cleans everything via the namespace path.

**Caveat — underlying cloud disk.** When a PVC is deleted, the backing `PersistentVolume` (cloud disk) is only freed if the StorageClass `reclaimPolicy` is `Delete` (the default for dynamic provisioning). Verify the cluster's default storage class uses `Delete`; a `Retain` reclaim policy would leave orphaned PVs/cloud volumes after PVC deletion.

## Decision deferred to layer 3 (note, not in scope)

- **Multi-replica history.** `replicas > 1` gives each pod its own PVC → per-replica sqlite, fragmented history (and sqlite is unsafe for shared concurrent writers regardless). Layer 3 should keep Redis, or accept per-replica history, for distributed agents.

## Out of scope

- Messaging sqlite `ConversationStore` implementation and the `STORAGE_TYPE=sqlite` switch (layer 3).
- Changing the default storage size from `10Gi`.

## Implications to accept

- Every deployment now provisions a real cloud PVC (default `10Gi` per replica) — real storage cost and a dependency on a working storage class in the cluster.
- StatefulSet rolling updates are ordered and lack Deployment surge semantics; updates may be marginally slower.
