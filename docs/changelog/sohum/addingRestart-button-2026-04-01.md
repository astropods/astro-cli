# Deployment Detail: History Tab, ActionPanel, and Cleanup

## Summary

Adds a deployment history tab to the agent detail view, a new `ActionPanel` design system component for contextual banners with CTAs, and a broad cleanup of dead code, hardcoded test data, useEffect anti-patterns, and stale status aliases.

## Design

### Deployment History Tab

The `DeploymentsTab` now renders a full deployment history pulled from a new server-side endpoint (`GET /api/v1/accounts/:account/deployments/:name/history`). Records are stored in a new `revisions` table in the deployment store and returned sorted descending by deploy time.

On the client, history records are merged with the live `AgentDeployment` object and grouped by `build_id` into collapsible `BuildHistoryGroup` rows. Each group shows the current config variant as the header and any re-deploys of the same build as sub-rows. The current deployment always appears as a pinned row above the history card.

Duration is computed between successive `deployed_at` timestamps; the current deployment shows elapsed time since deploy.

### ActionPanel Component

`ActionPanel` in `status-panel.tsx` is a neutral info banner with primary/secondary CTAs. The destructive confirmation flows through a Dialog (not an inline state replacement), keeping the panel layout stable. It supports a `dismissible` prop (renders an X button, hides on click) and a `ReactNode` title so pill badges can be embedded inline.

Used in `ActiveDetailView` to surface a "New build available" banner when `latestBuildId !== deployment.build_id`.

### useEffect Cleanup

The `pendingRestart`/`sawDeploying` state machine in `ActiveContainerAccordion` was replaced with `restartMutation.isPending || isGloballyRestarting`. TanStack Query's mutation state is the authoritative signal — no need to mirror it into local state or propagate it back up via `onRestartingChange`. This removed:

- 3 useEffects from `ActiveContainerAccordion`
- `onRestartingChange` prop chain through `DeploymentsTab` → `DeploymentHistoryTable` → `ActiveContainerAccordion`
- `restartingWorkloads` Set state in `ActiveDetailView`
- `isAnyWorkloadRestarting` accordion prop

The deployment history polling `setInterval`/`invalidateQueries` useEffect in `DeploymentsTab` was replaced with `refetchInterval` on `useDeploymentHistory`.

### Container Readiness Polling

`useDeployment` and `useDeploymentSuspense` now continue polling at 3s after a pause or resume until workload container `ready` flags match `deployment.replicas`. Previously, polling stopped as soon as `status` left a transitional string, leaving service accordions showing stale readiness counts until a manual refresh.

### Status Cleanup

- Removed the `"ready"` status alias from `DeployHistoryStatus` — inactive deployments now map to `"inactive"` which renders as "Inactive" in the history row, matching the header badge.

## Migration

No action required.
