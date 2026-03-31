## Summary

Inactive deployed agent cards on the Your Agents page were not clickable, preventing navigation to the deployment detail view for agents in the `inactive` state.

## Design

The `YourAgents` page conditionally set `href` on `DeployedAgentCard` based on deployment status, omitting it for `inactive`. Since `DeployedAgentCard` renders as a plain `div` (non-navigable) when `href` is absent, inactive cards were visually dimmed and unclickable.

The fix removes the `inactive` exclusion so all cards always receive an `href`. Inactive deployments link to the deployments tab (same as `error`, `pending`, and `undeploying`), which is the appropriate view for understanding why an agent is not running.

## Migration

No action required.
