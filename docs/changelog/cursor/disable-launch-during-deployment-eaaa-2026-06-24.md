# Disable Launch button during deployment

## Summary

The Launch button was previously enabled based solely on whether messaging was configured, allowing users to click it while a deployment was still provisioning, undeploying, or in an error state. Clicking Launch during these states resulted in an "agent is not ready" error message, creating a confusing user experience.

This change keeps the Launch button visible but disables it with a contextual tooltip explaining why it's temporarily inaccessible, providing clear feedback before users attempt to launch.

## Design

The Launch button appears in two places:
1. Agent detail page header (`AgentDetail.tsx`)
2. Dashboard agent cards (`DeployedAgentCard.tsx`)

Both locations now follow the same pattern: the button remains visible but is disabled when the deployment is not in an "active" state, with a tooltip explaining the current situation.

### Agent Detail Page

The header already fetches live deployment status via `useDeploymentStatus` (used by `AgentStatusToggle`). The fix adds:

```typescript
const isActive = statusData?.value === "active";
const launchDisabled = !isActive;
```

The Launch button is wrapped in a Tooltip with conditional content based on status:
- `"deploying"`: "Agent is still deploying. Launch will be available once deployment is complete."
- `"undeploying"`: "Agent is being undeployed. Launch is temporarily unavailable."
- `"error"`: "Agent is in an error state. Please check the deployment status."
- Other: "Agent is not ready. Launch will be available once the agent is active."

When disabled, the button uses `pointer-events-none` and reduced opacity, with click events prevented.

### Dashboard Agent Cards

`DeployedAgentCard` now accepts two new optional props:
- `launchDisabled?: boolean` — whether to disable the Launch button
- `deploymentStatus?: string` — deployment status for tooltip messaging

`DeploymentAgentCard` (the adapter that maps server data to card props) computes:

```typescript
const hasMessaging = isChatListEligible(deployment);
const canLaunch = hasMessaging;  // controls button visibility
const launchDisabled = deployment.status !== "Running";  // controls disabled state
```

The card's Launch button is wrapped in a Tooltip with status-specific messages matching the detail page. When disabled, the Button component's native `disabled` prop handles styling (reduced opacity, no pointer events).

### Utility Function

To avoid duplicating the tooltip message logic across components, a `getLaunchDisabledMessage` utility function was added to `deployment-utils.ts`:

```typescript
export function getLaunchDisabledMessage(
  statusValue?: DeploymentStatusValue | string,
): string {
  switch (statusValue) {
    case "deploying":
    case "pending":
      return "Agent is still deploying. Launch will be available once deployment is complete.";
    case "undeploying":
      return "Agent is being undeployed. Launch is temporarily unavailable.";
    case "error":
      return "Agent is in an error state. Please check the deployment status.";
    case "Stopped":
    case "inactive":
      return "Agent is paused. Resume the agent to launch.";
    default:
      return "Agent is not ready. Launch will be available once the agent is active.";
  }
}
```

Both `AgentDetail` and `DeployedAgentCard` call this function instead of maintaining separate conditional logic. Unit tests cover all status cases.

## Migration

No action required. The Launch button will now remain visible but disabled during transitional states, with clear tooltips explaining the situation. Users will no longer encounter "agent is not ready" errors from premature Launch clicks.
