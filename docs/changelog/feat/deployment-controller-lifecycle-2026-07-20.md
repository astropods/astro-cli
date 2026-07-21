# Deployment controller — Phase 2 (drive the lifecycle)

## Summary

Phase 2 of the event-driven deployment controller (`docs/plans/deployment-state-tracking.md`).
The controller now **drives the deployment lifecycle** from observed workload
health instead of the worker optimistically flipping to `active` the moment
manifests are accepted. `active` now means "all declared workloads are actually
running," and compute billing starts on that real transition.

## Design

**Ownership split via a new `deploying` status.** The deploy/wake-up workers
apply manifests and hand off at `provisioning → deploying` — they no longer set
`active`. The controller owns `deploying → active | failed`. Because the worker
only reaches `deploying` *after* apply succeeds, the controller never races a
mid-apply worker (it treats `pending`/`provisioning` as hands-off).

**Aggregation.** On each sync the controller reduces the per-workload
`deployment_workload_status` rows to a deployment decision:
- any workload `failed` → **failed**
- every *declared* workload (from `deployment_workloads`) observed and
  ready/complete → **active**
- otherwise → stay `deploying`

Requiring the full declared set to be present guards against marking active
before informer caches show every applied workload.

**Safe transitions.** Lifecycle writes go through a new compare-and-set
(`UpdateStatusIfCurrent`, `WHERE status = ANY(allowed)`): the controller only
drives `{deploying, active}`, so a concurrent stop/undeploy that already moved
the row wins the race — the controller can never resurrect a stopping deployment.
Transitions fire only on an actual phase change, so a healthy deployment
re-synced every resync doesn't spam `deployment_events`. `failed` is terminal
(no auto-recovery; a reapply re-enters `provisioning`).

**Deterministic failure.** Deployments are now built with
`progressDeadlineSeconds = 180`, so a stuck rollout (bad image, crash loop) trips
K8s's `ProgressDeadlineExceeded` — which the health derivation already maps to
`failed` — within ~3 minutes instead of the 600s default.

**Billing.** `StartBilling` moved out of the deploy/wake-up workers into the
controller's real `→ active` transition, so dead-on-arrival deploys are never
billed. `StopBilling` (undeploy) is unchanged.

**Status surfaces.** The server maps the new `deploying` status to the existing
transitional UI value in `/status` and `dbStatusToUIStatus`; the queen admin adds
it to the status filter/badge. The client consumes the mapped values and needs
no change.

## Migration

Additive `deploying` status value; no schema change (it's a string in the
existing `status` column) and no data migration — in-flight deployments simply
pass through `deploying` on their next deploy. No config or public API changes.
Deferred to later phases: pod-level failure-reason enrichment (ImagePullBackOff
etc.) and repointing the read path off the live K8s probe onto persisted status.
