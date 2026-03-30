## Summary

Several bugs caused the Pause/Resume buttons on the deployment detail page to behave incorrectly: double-clicks could trigger a 400 from the server, and the buttons wouldn't update after a successful pause or resume without a page refresh.

## Design

**Double-click on Pause caused a 400.** The `pausing` effect in `ActiveDetailView` had a logic error: the exit condition `isPaused || !isDeploying` always fired immediately for an `active` deployment (since `active` is not a deploying state), resetting `pausing` to `false` before the mutation resolved. This re-enabled the button while the stop request was still in-flight. A second click would hit the server with a 400 because the deployment was already `stopped`. The fix changes the exit condition to `isPaused` only, keeping the button disabled until the query reflects the new status.

**Buttons didn't update after pause/resume.** `DeployedAgentDetail` fetches via `useDeployment` (key: `['deployments', 'detail', id]`), but both `useStopDeployment` and `useWakeUpDeployment` only invalidated `deploymentKeys.all(account)`. The detail query was never touched, so the buttons stayed stale until a refresh. Both mutations now also invalidate `deploymentKeys.detail(deploymentId)`.

Additionally, `useWakeUpDeployment` had no `invalidateQueries` call at all — it only did an optimistic `setQueriesData` to set status to `pending`. Because TanStack Query doesn't re-evaluate `refetchInterval` on `setQueriesData`, polling never restarted after a resume, leaving the deployment stuck in `pending` state. Adding `invalidateQueries` after the optimistic update triggers a real fetch and restarts the interval.

## Migration

No action required.
