# Deployment controller — Phase 3 (read persisted) + Phase 4 (failure taxonomy)

## Summary

Builds on the Phase 2 controller (which drives `deployments.status` from observed
health). Phase 3 makes the API **read** that controller-maintained status instead
of probing Kubernetes per request; Phase 4 makes failures **fast and specific**
and removes the client-side status workarounds that existed because status used
to be unreliable.

## Design

**Phase 3 — read persisted status.** `GetDeploymentStatus` no longer does a live
`Deployments.Get` on every poll. When the DB says `active`, it trusts it (the
controller only made that transition after observing every declared workload
ready) and summarizes from `deployment_workload_status`. The per-request probe
and its `cluster_unreachable` / `ready_lag` fallbacks are gone. `/runtime` stays
live deliberately — it serves per-container detail (waiting/terminated reasons,
restart counts) that the persisted table doesn't hold and the pod grid renders.

**Phase 4 — pod-reason enrichment.** The controller now uses its pod informer:
for a workload that isn't ready, it classifies permanent pod failures
(`ImagePullBackOff`, `ErrImagePull`, `InvalidImageName`, `CreateContainerError`,
`CrashLoopBackOff`, `OOMKilled`) and marks the workload `failed` with that
specific reason — after a 90s grace / restart threshold that avoids
false-terminal on transient blips, but well before the 180s progress deadline.
The reason threads through: workload → deployment `error_message` → `/status`
details, so a failed deploy reads "ImagePullBackOff on X" rather than a generic
"ProgressDeadlineExceeded".

**Phase 4 — client workaround removal.** With status now controller-authoritative
and monotonic, the client drives straight off the current value: the
`RESUME_GRACE_MS` sliding grace window and the `use-stuck-deploy` age heuristic
are removed. `statusRefetchInterval` simply polls while transitional and idles
otherwise. The stuck-deploy banner remains but is now driven only by
server-surfaced "stuck"-severity events (current state), not a client timer.

## Scope notes

- **Cut:** stale-`pending`/`provisioning` recovery (a crashed worker / lost job
  can still hang there — deliberately not re-adding a periodic sweep now).
- **Dropped:** orphaned-namespace detection (a separate operational concern).
- **Deferred:** repointing the CLI `--wait` flag to poll `/status`.

## Migration

None — no schema, config, or public API changes. Behavior change: `/status`
reflects the controller's status directly, so it depends on the controller
running (it does, in `astro-worker`).
