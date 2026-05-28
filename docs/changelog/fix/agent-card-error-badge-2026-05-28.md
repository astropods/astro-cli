# Agents-page card: align Error badge with detail-page tile

## Summary

Paused / stopped agents on the agents grid were showing the red **Error** pill because the card treated any truthy `error_message` field as an error — even stale messages left over from a previous failed deploy attempt on a deployment that's now paused.

The detail-page `DeploymentTile` already gets this right: it only colors a deployment as an error when `mapDeploymentStatus()` returns `"error"`. The agents grid should match.

## Design

`DeploymentAgentCard` now derives `hasError` from `mapDeploymentStatus(deployment) === "error"` instead of `!!deployment.error_message`. That maps to the same rule the tile uses:

- backend status is `error` / `failed` / `crashloopbackoff`, **or**
- `ready === 0 && replicas > 0` (workload up, no healthy pods)

Paused deployments (`status=scaled_down`, `replicas=0`) fall through to `inactive` and no longer trigger the pill, even if a stale `error_message` is still on the row.

## Migration

None.
