# Delete Blueprint

## Summary

Owners could not remove agent blueprints (published agent packages) they no longer wanted to maintain. The only removal path was a direct API call. This adds a self-service delete flow scoped to the profile page blueprints section — deployed agents ("Your Agents") are unaffected.

## Design

A three-dot menu (`EllipsisVertical`) appears in the top-right corner of `AgentCard` only when an `onDelete` prop is provided. The `AccountProfile` page passes `onDelete` for members viewing their own blueprints; it passes `undefined` for non-members and for deployed-agent cards so the menu never appears there.

Clicking "Delete" opens a `DeleteAgentDialog` (a `ConfirmationDialog` wrapper) that requires the user to type the agent name before confirming. On confirmation the `useDeleteAgent` mutation calls `DELETE /api/v1/agents/:account/:name`, then invalidates both the per-account and global agent query caches so the card disappears without a manual refresh.

Backend: a new `DeleteAgent` handler on `astro-server` calls `agentindex.Delete(accountID, name)` and responds `204 No Content`. The route is registered under the existing `agentWriteRoutes` group (bearer auth required). Four Go unit tests cover success, not-found, DB error, and missing-account-context cases. UI copy uses "blueprint" throughout.

## Migration

No migration required. No existing data is affected. The new endpoint is additive.
