# Graceful not-found for deleted deployments

## Summary

Opening an agent detail page for a deployment that no longer exists — for example, clicking a stale link on the Insights page after the agent was deleted — crashed the page instead of explaining what happened. The detail layout couldn't build its context, and the child tabs blew up trying to read the deployment from a null context. Deleted or unknown deployments now show a clear "Deployment not found" state.

## Design

The agent detail layout builds a context object from the loaded deployment record and hands it to the tab `Outlet`. When the record 404s (deleted or unknown id), that context is null, and every tab destructures the deployment from it, so they threw.

The layout now renders the `Outlet` only when the context exists. Once the record query has settled with no deployment, it renders a `Deployment not found` panel (with a link back to the agents list) instead of mounting a tab against a null context. While the query is still loading, it renders nothing rather than flashing the not-found state.

This fixes the crash at its root — any route into a deleted deployment, not just the Insights link that surfaced it.

## Migration

None.
