# Remove Delete agent from the agent-card kebab menu

## Summary

On the agents grid, every deployed agent card exposed a destructive **Delete agent** action through its kebab menu — one click away from a dense list, with no permission gate. The action was there in name only: the server still authorizes the underlying `POST /api/v1/undeploy` on bare account membership, so any member could delete via the CLI regardless of what the UI showed. Real enforcement will land with the platform-wide move to fine-grained access control. In the meantime, this removes the prominent grid-level affordance.

## Design

The "Delete agent" item is removed from the kebab menu on the deployed agent card entirely. Users who need to delete still have a clear path through the agent detail page (and through the configure Danger Zone). The `onDeleteRequest` prop chain comes out with it: `DeployedAgentCard` no longer accepts it, the `DeploymentAgentCard` adapter no longer forwards it, and `DashboardAgentsSection` no longer holds delete-target state or mounts the confirmation dialog.

The agent detail page and the configure Danger Zone are untouched — they remain in place until fine-grained access control takes over the permission story end-to-end.

## Migration

None.
