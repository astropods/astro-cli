## Summary

Agents in `pending` or `provisioning` state could not be deleted — the undeploy endpoint rejected any status except `active`, `scaled_down`, `stopped`, and `failed`, returning "active deployment not found". This made it impossible to cancel a deployment that was still starting up.

## Design

The undeploy handler's status guard was inverted: instead of listing the two states that genuinely cannot be undeployed (`undeploying` and `undeployed`), it listed the four that could. Adding `pending` and `provisioning` to the allowlist fixes the symptom, but creates a race on `provisioning`: the deploy worker reads status, sets `provisioning`, runs `Apply`, and then unconditionally sets `active` — overwriting any `undeploying` transition that happened concurrently.

The race is closed in the deploy worker by re-reading the deployment after `Apply` completes. If the status is no longer `provisioning` (e.g. it was set to `undeploying` mid-flight), the worker skips the `active` transition and returns cleanly. The `Teardown` call in the undeploy worker is already idempotent (404 → nil), so cancelling before K8s resources exist is safe.

The `pending` case has no race: the deploy worker checks `status == pending` as its first action and skips if the status has changed, so setting `undeploying` before the worker starts is sufficient.

## Migration

No action required.
