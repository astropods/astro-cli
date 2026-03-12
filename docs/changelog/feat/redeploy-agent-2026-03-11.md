# Redeploy Agent — Client Configure Pages

## Summary

Adds a tabbed "Configure" experience for deployed agents, replacing the flat settings page. Users can reconfigure deployment variables and adapters (with pre-filled values from the existing deployment), and delete deployments from a dedicated danger zone tab. Routes now use deployment IDs instead of agent names, fixing collisions when multiple deployments share the same agent.

## Design

**Routing overhaul:** Deployment detail and configure routes use `:deploymentId` instead of `:agentName`. A centralized `lib/routes.ts` provides `deploymentPath()` and `deploymentConfigurePath()` helpers used across all pages that link to deployments.

**Configure layout:** `DeployedAgentSettings.tsx` is a React Router v7 layout route that fetches the pre-filled deployment template and passes it to child routes via `<Outlet context>`. Child routes are `ConfigureDeployment` (form with floating action bar) and `ConfigureDangerZone` (delete with confirmation dialog). A shared `ConfigureContext` type lives in `pages/configure/types.ts`.

**Form reuse:** Deploy form fields were extracted into `DeployFormFields.tsx`, shared by the install page and configure page. `useDeployForm` gained `initialValues`, `skipTemplateFetch`, and a `reset()` method for the reconfigure use case. `extractInitialValues.ts` converts a pre-filled template into form state, with `Array.isArray` guards for adapter parsing.

**Change tracking:** `useChangeTracking` accepts explicit initial and current state (no more ref-based snapshot), enabling the floating `DeployFormActionBar` to show pending update counts and distinguish cosmetic vs. redeploy-required changes.

**Server:** `UpdateDeploymentFull` now persists `display_name` changes. Secret variables are stored as plaintext when no encryptor is available, allowing the pre-filled template endpoint to return values for reconfiguration.

## Migration

No database migration. Client routes changed from `/:account/agents/:agentName` to `/:account/agents/:deploymentId` and from `.../settings` to `.../configure/deployment`. Old bookmarks will 404.
