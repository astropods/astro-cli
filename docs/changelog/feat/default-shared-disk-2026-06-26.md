# Default shared disk for every deployment

## Summary

Disk used to be opt-in: an agent only got persistent storage if its spec set `agent.volume`, otherwise it ran as an ephemeral Deployment and lost all on-disk state across restarts. This made the platform's storage story inconsistent with compute and memory (always provisioned) and left the messaging sidecar with nowhere durable to keep conversation history.

Now **every agent gets a persistent disk by default** — a 5Gi RWO volume mounted at `/data` — and that disk is **shared with the co-located messaging sidecar**, laying the groundwork for messaging to persist history (sqlite) without any per-agent configuration. There is no longer an enable/disable choice for persistence.

## Design

**Disk is guaranteed at the apply choke point, not just at template generation.** The deploy path loads the stored spec JSON verbatim and applies it — it does not re-run the template generator — so defaulting only at generation would silently skip redeploys of pre-existing specs. The guarantee instead lives in `normalizeAgentStorageDefaults`, run at the top of `ApplyDeploymentSpec` (the single path every deploy passes through): if `agent.volume` is empty it applies the default mount + storage. Idempotent; an explicitly requested mount/size is left untouched. The template generator still seeds the same default so the Configure UI shows the effective values, but it is not what the guarantee rests on.

**Agents are now always StatefulSets.** Because the volume is guaranteed, the stateless-Deployment branch for agents (and the `volume != ""` conditionals guarding it across the applier and orphan cleanup) is removed. Orphan cleanup now always expects the agent StatefulSet, which means a legacy agent's stale Deployment is correctly torn down on the next reconcile.

**The messaging sidecar shares the agent's disk.** Agent and messaging run in the same pod, so the sidecar mounts the same `data` PVC — at `/data` under `subPath: messaging`, giving it an isolated subtree on the shared disk (ReadWriteOnce is sufficient; no ReadWriteMany needed).

**Local-mode hardening stays on for the agent.** Security relaxation on local kind clusters is meant only for third-party provider containers (redis, qdrant, etc.). The StatefulSet builder previously relaxed *every* StatefulSet in local mode, since it only ever built providers — now that the agent is a StatefulSet, that gate is made provider-aware (matching the Deployment builder), so the agent pod and its messaging sidecar remain hardened even locally while providers still relax.

**PVC lifecycle.** The agent StatefulSet retains its PVC on delete (`WhenDeleted: Retain`, `WhenScaled: Delete`) so data survives a redeploy that recreates the StatefulSet. Full teardown still reclaims the disk: the deleter explicitly removes PVCs and then the namespace (which cascades), independent of the retention policy. (Freeing the underlying cloud volume depends on the StorageClass `reclaimPolicy` being `Delete`.)

**Frontend.** The deploy form drops the enable/disable "Persistent volume" toggle entirely — storage is always provisioned. Storage size lives in "Advanced sizing" (default 5Gi), and the mount path is an optional advanced field defaulting to `/data`, preserving the prior ability to point the disk at a custom path.

## Migration

None required. Existing agents that were deployed as ephemeral Deployments flip to StatefulSets on their next redeploy — the stored spec is normalized at apply time, the StatefulSet is created, and the old Deployment is removed as an orphan. This is a one-time pod recreate with no spec changes and no user action. Agents that already declared a volume are unaffected.
