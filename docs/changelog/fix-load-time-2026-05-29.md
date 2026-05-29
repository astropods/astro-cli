# fix-load-time — 2026-05-29

## Summary

`GET /deployments` was making per-account Kubernetes API calls (Namespace, Deployment, StatefulSet, Ingress) on every request to enrich agent cards. These calls were the dominant latency driver for the Agents page. The agent card grid only needs a handful of scalar fields, none of which require live K8s reads.

## Design

**Server (`ListDeployments`)**: K8s enrichment is removed entirely. Deployments are now built from the DB only via `agentDeploymentFromDB`. Messaging URLs — previously fetched from K8s Ingresses — are read in a single batch query against `deployment_ingresses` (joined through `deployment_sidecars` → `deployment_services` → `deployment_ingresses`). The `listAstroDeploymentsLight` helper and its K8s enrichment loop are deleted.

**Response shape**: A new `AgentDeploymentSummary` struct replaces the full `AgentDeployment` in the list response. It contains only the fields the card grid actually uses: `id`, `name`, `display_name`, `avatar_colors`, `build_id`, `latest_build_id`, `status`, `external_urls`, `created_at`, `updated_at`. The full `AgentDeployment` type is preserved for the single-deployment detail endpoint. The `status` field carries DB-native values (`pending`, `provisioning`, `undeploying`, `error`) — `agentDeploymentFromDB` maps `StatusFailed` → `"error"` so the client error badge contract is unchanged.

**Client**: `AgentDeploymentSummary` TypeScript interface mirrors the trimmed server shape. `getMessagingEndpoint`, `useAgentFilters`, and `useDeploymentSummaryMaps` accept both `AgentDeployment` and `AgentDeploymentSummary`. The `refetchInterval` in `useDeployments` continues to poll on transitional DB statuses (`pending`, `provisioning`, `undeploying`). `DeploymentAgentCard` reads `deployment.status === "error"` directly instead of going through `mapDeploymentStatus`.

**Tests**: Server test pins `StatusFailed` → `"error"` mapping in `AgentDeploymentSummary`. Client tests pin the error badge contract. `GetMessagingURLs` is covered with four cases (URL present, absent, batch filtering, empty input). Test fixtures updated to reflect real server-emitted status values.

## Migration

No action required.

**Known behavioral change**: the error badge on the agents grid now reflects DB-only state. Runtime failures that the DB doesn't know about — crash-looping pods, OOM kills, or any condition the reconciler hasn't yet escalated to `StatusFailed` — will not surface on the grid. They remain visible on the agent detail page, which still reads live K8s state. Previously the grid made live K8s calls on every request and would catch these immediately.
