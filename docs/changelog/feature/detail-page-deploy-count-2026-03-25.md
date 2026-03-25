## Summary

Deploy count was missing from the agent detail page sidebar and "Often Used Together" cards. The `AgentMetrics.deploy_count` field was already being returned by the API and the UI components already supported rendering it — it just wasn't being passed from the page.

## Design

`AgentDetail.tsx` now passes `agent.metrics?.deploy_count` as `installs` to both `AgentDetailSidebar` and the mobile `SidebarCard`. It's also included as `deployCount` when building the `recommendedAgents` array so the "Often Used Together" cards show real deploy numbers instead of 0.

## Migration

No migration required.
