# Remove Legacy Operator/Dashboard

## Summary

Removes the deprecated Operator UI (also known as "Dashboard") from astro-client. All of its functionality — agent builds, deployments, observability, and playground chat — has been superseded by the current agent management pages (`AgentDetail`, `DeployedAgentDetail`, `InstallAgent`, etc.).

## Design

Deleted the `pages/legacy/` directory (3 pages: `OperatorOverview`, `DeployPage`, `AgentPage`) and the `components/operator/` directory (9 components including `DeploymentCard`, `PlaygroundChat`, `ObservabilityTab`, and various modals). Removed the `/operator`, `/operator/deploy/:account/:name`, and `/u/:account/:agent` routes, along with the "Dashboard" nav link in the header.

Operator-only query hooks (`useConfigMapData`, `useSecretKeys`, `useObservabilitySummary`, `useObservabilityTraces`) and their key factories were removed. Shared hooks used by both Operator and current pages (`useAgent`, `useDeployments`, `useRestartPod`, etc.) and all API client methods were kept intact.

## Migration

No action required. Users visiting `/operator` will now see a 404.
