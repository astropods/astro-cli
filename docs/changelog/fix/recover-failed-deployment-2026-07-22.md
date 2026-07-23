# Recover deployments that self-heal after a failure

## Summary

A deployment whose pods later recovered (crash loop settled, a fixed image
finally pulled) stayed stuck at `failed` forever, even though `/runtime` showed
every workload `Running` and ready. The failure verdict is derived from live
cluster observation, so it must be able to un-fail when observation turns
healthy again.

## Design

The deployment controller's `driveLifecycle` reduces observed workload health to
a deployment-level status. It previously treated `failed` as terminal: any sync
on a `failed` deployment returned early, so a recovery was never noticed.

`failed` is now a **drivable** state alongside `deploying` and `active`:

- When every declared workload is observed ready again, the controller drives
  `failed → active` via a compare-and-set (allowed current: `deploying` or
  `failed`). The transition clears the stale `error_message`, records a clean
  `active` event, and starts billing.
- `stopped` / `undeploying` / `undeployed` remain hands-off, and the
  compare-and-set prevents resurrecting a concurrently stopping deployment.
- Recovery is event-driven — a pod becoming ready fires an informer update that
  drives the transition within seconds; the periodic resync is the backstop.

An Apply-time failure (bad spec) has no healthy workloads, so it does not
spuriously recover; it still requires a real redeploy.

## Migration

None. Existing stuck-`failed` deployments recover automatically on the next sync
once their workloads are healthy.
