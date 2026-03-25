# Rename Agent → Blueprint in astro-client

## Summary

The codebase used "Agent" to mean two different things: the template/spec pushed from the CLI (a blueprint) and a running deployment of that template. This renames all blueprint-layer symbols, files, and directories from `Agent*` to `Blueprint*` to cleanly separate the two concepts. Deployment-side naming (`DeployedAgentCard`, `AgentDeployment`, `useDeployAgent`, etc.) is untouched.

## Design

**Types** (`api.ts`):

- `Agent` → `Blueprint`
- `AgentVersion` → `BlueprintVersion`
- `AgentSpec` → `BlueprintSpec`
- `AgentCardData` → `BlueprintCardData`
- `AgentCardAuthor` → `BlueprintAuthor`
- `AgentMetrics` → `BlueprintMetrics`
- `AgentsListResponse` → `BlueprintsListResponse` (field `agents` → `blueprints`)
- `AgentSummary` → `BlueprintSummary`

API client methods:

- `listAgents` → `listBlueprints`
- `getAgent` → `getBlueprint`
- `archiveAgent` → `archiveBlueprint`

API endpoint URL strings are unchanged — the rename is client-side only.

**Query layer** (`keys.ts`, `blueprints.ts`):

- `agentKeys` → `blueprintKeys`
- `useAgents` → `useBlueprints`
- `useAccountAgents` → `useAccountBlueprints`
- `useAgent` → `useBlueprint`
- `useArchiveAgent` → `useArchiveBlueprint`
- File renamed `agents.ts` → `blueprints.ts`

Hooks that bridge blueprint→deployment (`useDeployAgent`, `useDeploymentTemplate`, `usePrefilledDeploymentTemplate`) keep their names.

**Components**:

- `AgentCard` → `BlueprintCard`
- `AgentIdentity` → `BlueprintIdentity`
- `ArchiveAgentDialog` → `ArchiveBlueprintDialog`
- `AgentListView` → `BlueprintListView`
- `AccountAgentsList` → `AccountBlueprintsList`
- `RecommendedAgents` → `RecommendedBlueprints`
- `FindAgentsWizard` → `FindBlueprintsWizard`
- Directory `agent-detail/` → `blueprint-detail/` with all five `AgentDetail*` components renamed to `BlueprintDetail*`

**Pages**:

- `AgentDetail` → `BlueprintDetail`
- `InstallAgent` → `DeployBlueprint`
- `RequestAgent` → `RequestBlueprint`

**Utilities**:

- `agent-utils.ts` → `blueprint-utils.ts`
- `getAgentDescription` → `getBlueprintDescription`
- `getAgentCategories` → `getBlueprintCategories`
- `getAgentReadme` → `getBlueprintReadme`
- `getAgentAuthors` → `getBlueprintAuthors`
- `getAgentCapabilities` → `getBlueprintCapabilities`
- `getAgentIntegrations` → `getBlueprintIntegrations`

**Props/params** (`agentName` → `blueprintName`):

- `ArchiveBlueprintDialog`
- `BlueprintDetailBreadcrumb`
- `DeployFormFields`
- `pod-utils.ts`

Route URL paths, API endpoint strings, and the `Account.agents` server-contract field are intentionally unchanged.

## Migration

No migration required. This is a client-side-only rename with no API contract changes.
