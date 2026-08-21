# Stop crash-looping deployments flipping between active and failed

## Summary

A deployment whose container is in `CrashLoopBackOff` alternated between `active`
and `failed` roughly once every two minutes, for as long as the crash loop
lasted. Each flip wrote a `deployment_events` row and restarted compute billing,
so the event feed filled with noise and billable time was silently dropped.

The cause is that `CrashLoopBackOff` is not a steady state. Between crashes the
kubelet restarts the container, the pod passes its readiness probe, and the
Deployment reports `readyReplicas == desired`. At that instant the workload looks
healthy by every counter the controller reads.

## Design

### Readiness now has to survive the backoff ceiling

`enrichFromPods` used to return early for any workload that already derived
`ready`, on the reasoning that a ready workload cannot have a wedged pod. That
holds for image-pull and config failures, which keep a pod out of the ready set
for their whole duration. It does not hold for a crash loop, where the only
durable evidence lives in the container's restart count, and the restart count
was read on exactly the passes where the workload was not ready.

A ready workload now runs a second, narrower check. A container fails it when
both are true:

- its restart count is past `crashLoopRestartLimit`
- its current run is younger than `crashLoopStableWindow` (5 minutes)

Five minutes is the kubelet's `CrashLoopBackOff` delay ceiling. A container that
has run longer than the maximum backoff has out-run the loop, so its restart
history stops counting against the workload and the deployment goes `active`.
This is the hysteresis the lifecycle was missing: readiness alone can be
momentary, readiness plus a completed stable run cannot.

The check reads only pods whose `Ready` condition is true. A crash-looping pod
that is currently backing off contributes nothing to the workload's ready count,
so it cannot fail a deployment on its behalf. That is what keeps rollouts
correct: when a healthy new pod is ready and a doomed old replica is still
listed under the same selector, only the new pod is consulted.

Recovery is unaffected in the common case. Shipping a fixed image creates fresh
pods with a zero restart count, which never enter the gate. The window only
delays a deployment that self-heals in place, and 5 minutes of `failed` there is
a better answer than a status that changes every other minute.

### Billing keeps its anchor across a recovery

`StartBilling` runs on every transition into `active`, including
`failed` -> `active`. Its upsert set `last_emitted_at = now` unconditionally.
When a deployment failed and recovered between two heartbeats, the heartbeat
never saw the failure, and the reset moved the anchor past active time that had
not been billed yet. That interval was gone.

The upsert now preserves `last_emitted_at` when the row is already
`billing_active`, and resets it only when billing had genuinely stopped:

```sql
last_emitted_at = CASE WHEN deployment_billing_state.billing_active
                       THEN deployment_billing_state.last_emitted_at ELSE $3 END
```

An active row's anchor belongs to the heartbeat, which advances it only after
emitting the CU-hours it covers. An inactive row was already billed out by
`reconcileStale` or `reconcileStopped`, so resetting it is what stops the
stopped period being charged.

One consequence is worth naming: a failure that starts and ends inside a single
heartbeat window is now billed as active time. The pods held their CPU and memory
requests throughout, and the alternative is to keep losing the surrounding
billable minutes, so the trade favors the anchor.

Event-feed noise needed no separate fix. `driveLifecycle` already writes only on
a real phase change, so a deployment that stays `failed` writes one row.

## Migration

None. No schema, config, or manifest changes. Deployments currently oscillating
settle on `failed` and report `active` once their container completes a 5-minute
run.
