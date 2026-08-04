# Cancel an in-progress deployment

## Summary

A deployment that hangs while coming up (for example a pod referencing a missing ConfigMap/Secret, or an unschedulable pod) sat in "deploying" and blocked all redeploys for that agent until the Kubernetes progress deadline eventually failed it, roughly half an hour later. Users now have an escape hatch: they can cancel an in-progress deployment.

## Design

A new `POST /deployments/:id/cancel` endpoint aborts a deployment while it is still coming up. It is allowed only from the transitional states (`pending`, `provisioning`, `deploying`); anything else returns 400. It marks the deployment `failed` and returns immediately. The write is a compare-and-set gated on those transitional states, so if the controller drives the row to `active` between the read and the write, cancel is a no-op rather than clobbering a now-active deployment back to `failed` (which, since `failed` is drivable, would re-activate and start billing a second time). The action is audited as `deployment.cancel`, distinct from `deployment.stop`.

Cancel does not touch the workloads. The main API process never drives Kubernetes directly (live cluster mutations belong to the deployment controller), and tearing a workload down on cancel could kill a still-healthy serving pod from an earlier revision and cause downtime. `failed` is drivable, not terminal: if the workloads turn out to be healthy, the controller drives the deployment back to `active` (and bills it) on its next sync, so a cancelled-but-healthy rollout is never left serving traffic unbilled; a genuinely stuck deploy stays `failed` and surfaces the recovery banner.

On the client, the deployment history menu shows "Cancel deployment" while the deployment is deploying (in place of Pause, which the server rejects mid-deploy). It calls the cancel endpoint, flips the record to `failed`, and seeds the status query with the failed state so the menu leaves the "Cancel deployment" state immediately. The stuck-deploy banner then takes over, from which the user fixes config and redeploys, or rolls back.

## Migration

None. New endpoint and a menu item; no changes to existing behavior.
