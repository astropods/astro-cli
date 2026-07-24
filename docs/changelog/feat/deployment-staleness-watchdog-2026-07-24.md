# Deployment staleness watchdog

## Summary

A deployment could sit in a non-terminal in-progress status (`pending`,
`provisioning`, `deploying`) indefinitely. The only existing timeout was
Kubernetes `progressDeadlineSeconds` (180s), which bounds **Deployment-kind**
rollouts only. Stalls with no time bound included: StatefulSet/Job pods that
never schedule or whose PVC never binds (no `progressDeadlineSeconds` equivalent),
and a deploy worker that crashed mid-apply — a River retry no-ops once the row
has left `pending`, stranding it in `provisioning` forever. Such deployments
showed a permanent "deploying" spinner with no resolution.

## Design

Two independent layers.

**Time-based watchdog (catch-all).** A new periodic River job
(`deployment.staleness_sweep`, every 5m) fails any deployment whose
`status_changed_at` has exceeded a per-status deadline:

| Status | Deadline | Rationale |
|--------|----------|-----------|
| `deploying` | 30m | Backstops the kinds K8s `progressDeadlineSeconds` never covers. |
| `provisioning` | 15m | Only the worker holds this; overrun means it died mid-apply. |
| `pending` | 15m | Deploy job never started (lost / worker down). |

The age check lives in the `UPDATE ... WHERE status_changed_at < NOW() - interval`
predicate, so the flip is atomic — a deployment that legitimately transitions
between sweeps can't be clobbered. Because `status_changed_at` resets on every
transition, it measures time-in-current-phase, so a slow-but-advancing rollout
(`pending → provisioning → deploying`) is not tripped early.

The flip to `failed` is **soft**: `failed` is a drivable state, so the existing
event-driven deploy controller drives `failed → active` once workloads later
observe healthy. The watchdog unsticks the UI without foreclosing recovery, and
never tears down resources (matching the apply-failure path).

**Event-path fast-fail (common cases).** The deploy controller's pod classifier
now recognizes a standing `PodScheduled=False/Unschedulable` condition past the
90s grace window as a failure. This covers unschedulable pods (no fit-able node)
and unbound PVCs (which surface as unschedulable) — and because pod enrichment
already runs for Deployments, StatefulSets, and Jobs, all three now fail fast
with a specific reason instead of waiting out the watchdog deadline.

Also removed the dead `last_reconcile_at` field from the admin
`GetDeploymentJobs` response — it queried a `kind = 'reconcile'` River job that no
longer exists (the periodic reconciler was replaced by the informer resync).

## Migration

None. No schema changes (`status_changed_at` and `deployment_events` already
exist); the watchdog and event-path changes are additive.
