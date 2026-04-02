## Summary

Fixes and improvements to the agent dashboard to bring it to parity with the My Agents page, correct all-time stats display, and fix back navigation for org accounts.

## Design

**Reveal overlay after deploy**

`DeployBlueprint` navigates to `/dashboard` with reveal state after a successful deploy, but `AgentDashboard` had no `LiveRevealOverlay` logic — that only existed in `YourAgents` (`/agents`). The fix ports the same pattern: read `revealDeploymentId` / `revealAgentName` from router location state, build an optimistic deployment stub while polling catches up, and render the overlay. While the overlay is open, the newly-deployed card in `DeployedAgentsSection` renders as a skeleton via a new `skeletonDeploymentId` prop.

**Back button preserves org account**

Clicking "View deployment" from the overlay passed `{ fromAgents: true }` in router state, causing `DeployedAgentDetail` to use the hardcoded `dashboardPath` (`/dashboard`) as the back destination — dropping `?account=my-org` for org accounts. The fix stores the full source path as `backPath` in state. `DeployedAgentDetail` uses it when present, falling back to `dashboardPath` for navigations that don't carry it.

**All-time stats**

TOTAL TOKENS and TOTAL REQUESTS were fetching a rolling 24-hour window, showing daily counts rather than lifetime totals. The fix replaces the two time-windowed queries (today + yesterday) with a single all-time query (no `start_time`/`end_time`), and removes the trend indicators which no longer have a meaningful reference period.

**Server-side skeleton count**

The dashboard loader now pre-fetches the deployment count for the active account — using the `?account=` query param when present, falling back to the personal account — so the correct number of skeletons render immediately on first load rather than a fixed count of four.

## Migration

No action required.
