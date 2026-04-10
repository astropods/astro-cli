# Logs Tab: Dedicated Tab with Multi-Tab Log Viewer

## Summary

Moves the deployment log viewer out of the Deployments accordion into a dedicated "Logs" tab in the agent detail view. Logs are removed from the accordion entirely. A reusable `LogViewer` component is extracted for use in any context.

## Design

**Logs tab:** A new "Logs" tab sits after Monitor and Deployments in the agent detail nav. It opens with an empty state prompting the user to select a container. Containers are opened as tabs via a `+` dropdown grouped by service, each closable with `×`. Open tabs persist when switching between Monitor, Deployments, and Logs. Switching to a different deployment resets to empty.

**Restart in Deployments:** A three-dot menu on each service row in the Deployments accordion provides a "Restart" action that deletes the pod, causing Kubernetes to recreate it. Services with no variables or domains are no longer expandable.

**Reusable `LogViewer`:** The log card (toolbar, filters, search, time range, copy) is extracted to `src/components/LogViewer.tsx` with no deployment-specific logic, covered by a Storybook story and test suite.

## Migration

No backend or query key changes. Nothing required from users.
