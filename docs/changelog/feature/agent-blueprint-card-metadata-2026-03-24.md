# Blueprint Card Metadata Updates

## Summary

Updates the metadata displayed on agent blueprint cards to surface deploy count and account identity instead of lifetime message count.

## Design

**Frontend (astro-client)**
- **Deploy count** replaces lifetime message count in the default card footer, displayed as "X deploys".
- **Account avatar** (16px, round) added to the left of the account name in the default card footer using the existing `UserAvatar` component.
- **oftenUsedTogether variant** updated to show deploy count and account name separated by a bullet divider (`•`).
- **`deploy_count`** added as an optional field to the `AgentMetrics` type in `api.ts`.

**Backend (astro-server)**
- **`BulkDeploymentCounts`** added to `deploymentstore`, querying total deployment count per agent name for a given account — using the same nil-safe pattern as `BulkMessageCounts`.
- **`deploy_count`** added to the `AgentMetrics` response struct and populated in `ListAgents`, `ListAccountAgents`, and `GetAgent` handlers using the same lazy-loading bulk-fetch pattern as message counts.

## Migration

No migration required. No schema changes — `deploy_count` is derived from the existing `deployments` table.
