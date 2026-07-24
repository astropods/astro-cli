# StatefulSet rollout unblocks pods wedged on the prior image

## Summary

A StatefulSet-backed workload (agent with persistent storage, self-hosted knowledge/model providers) could get permanently stuck on an old image after a redeploy. If the running pod was wedged in `ImagePullBackOff` (e.g. its previously-resolvable image tag was pruned or otherwise disappeared) when a new deploy changed the image, the deployment never recovered: it kept trying to pull the old, now-missing image indefinitely.

The cause is a Kubernetes StatefulSet `RollingUpdate` guarantee: the controller will not replace a pod that is not Running-and-Ready. A pod stuck in `ImagePullBackOff` is *Pending*, not *Failed*, so the controller blocks *on* it rather than evicting it. The server updated the StatefulSet template (new image, new update revision), but the pod stayed on the prior revision forever — the new image never reached it.

The deploy controller already *detected* this state (marking the workload failed with the pod's reason) but nothing *remediated* it, and the staleness watchdog only flips the deployment row's status without touching resources.

## Design

The fix lives in the deploy controller's reconcile loop, not the deploy path — so it is *curative*, recovering deployments that are already deadlocked without requiring a new deploy. On each sync, for every StatefulSet the controller now evicts pods that cannot roll on their own:

- **Trigger is the stale-revision signature**: the StatefulSet's `updateRevision` differs from its `currentRevision` (a rollout is in flight) and a pod is on a revision older than `updateRevision` while wedged on a permanent image-pull/create wait. That pod is exactly the one the RollingUpdate refuses to replace.
- **Gating on stale revision prevents churn**: a pod wedged on the *current* update revision (a genuinely bad new image) has no newer revision to roll to, so it is left alone rather than deleted and recreated in a loop.
- Only permanent waits (`ImagePullBackOff`, `ErrImagePull`, `InvalidImageName`, `CreateContainerError`) qualify; healthy pods are never touched.
- Eviction is best-effort — a delete failure is logged and retried on the next resync.

Because a deploy already enqueues an immediate reconcile (and there is a periodic resync backstop), this covers both a fresh deploy that changes the image and a deployment that deadlocked earlier: the wedged pod is deleted, the controller recreates it on the update revision, and the rollout completes.

## Migration

None. Deployments already wedged before this change recover automatically on the next reconcile — no manual pod delete required.

---

# Admin console: deployment detail overhaul

## Summary

The Queen deployment-detail page duplicated status/cluster/build facts across a stack of banners and cards, hid sidecar containers entirely, and showed logs/env inline. It's reworked around a lifecycle state machine and per-container cards, with the redundancy removed.

## Design

**Lifecycle graph.** A new panel at the top renders the deployment state machine — the happy-path spine (`pending → provisioning → deploying → active`) plus exception/teardown off-ramps (`failed`, `paused`/`suspended`, `undeploying`, `undeployed`) — and marks where the deployment currently sits. Position is driven by the DB status, with the one runtime nuance the product UI also surfaces: DB `active` but workloads not ready renders as still-deploying (amber). The failure reason is folded into this panel instead of a separate red banner.

**De-duplication.** Cluster/placement info collapsed from ~5 mentions to a single line; the 5-card K8s summary became a one-line snapshot; the metadata cards became one inline row (moved to the top). Redundant transitional banners and the standalone Status/Build-ID cards were dropped.

**Pods by container.** Pod info was smeared across parallel per-field sections and truncated the image. Each container is now one card (state, restarts, full image, resources, security, mounts, envFrom) with its own Logs/Env actions. Logs/env open in a modal rather than expanding inline.

**Sidecar visibility (server).** The admin pod-info builder only read regular containers, so native sidecars (the messaging container, added as a `restartPolicy: Always` init container on StatefulSet agents) were invisible. It now also walks `InitContainers`/`InitContainerStatuses`, tagging each container `init`/`sidecar` so the UI can badge it.

**Dead code.** Removed the unused admin `SetAdapters` path end to end (frontend hook, Queen route/handler, server RPC, gRPC service plumbing, message types).

## Migration

None for the UI. The sidecar-visibility change is server-side, so sidecars appear only after `astro-server` is redeployed.
