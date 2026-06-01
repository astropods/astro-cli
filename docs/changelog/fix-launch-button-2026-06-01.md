## Summary

The Launch button on deployment cards was visible even when a deployment was not running (e.g. stopped, deploying, or in error). Clicking it would open a URL for a service that wasn't up yet.

## Design

The list deployments endpoint returns a `status` field on each deployment summary, mapped from DB state by `agentDeploymentFromDB`: `"Running"` when active, `"Stopped"` when scaled down or stopped, `"pending"` when provisioning, `"undeploying"` when tearing down, and `"error"` on failure.

`DeploymentAgentCard` now gates `launchUrl` on `status === "Running"`. When the status is anything else, `launchUrl` is `undefined` and the card falls back to the existing "Manage agent" button — no new UI needed.

`DeploymentSummaryStatus` is introduced in `api.ts` as a typed union of the possible values, narrowing `AgentDeploymentSummary.status` from `string`.

## Migration

No action required.
