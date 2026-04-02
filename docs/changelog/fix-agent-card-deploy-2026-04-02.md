## Summary

Two fixes to the agent dashboard: the deploy reveal overlay was not appearing after deploying an agent, and the stats cards were showing daily metrics instead of all-time totals.

## Design

**Reveal overlay after deploy**

When a deployment succeeds, `DeployBlueprint` navigates to `/dashboard` with reveal state (`revealDeploymentId`, `revealAgentName`, etc.) in router location. The `LiveRevealOverlay` logic — which reads that state, builds an optimistic deployment stub while polling catches up, and renders the agent card celebration modal — existed only in `YourAgents` (`/agents`). `AgentDashboard` (`/dashboard`) was added without it, so the overlay never appeared.

The fix ports the same reveal pattern into `AgentDashboard`: read reveal state from location, compute `revealDeployment` (preferring live data from `useDeployments` once it arrives, falling back to an optimistic stub), and render `LiveRevealOverlay` with dismiss and view-deployment handlers.

**All-time stats**

The TOTAL TOKENS and TOTAL REQUESTS cards were fetching a 24-hour window and a prior-24-hour window to compute a value + trend indicator. The value was therefore a daily count, not a lifetime total, and the trend arrows added noise without clear context. The fix replaces the two time-windowed queries with a single all-time query (no `start_time`/`end_time` params) and removes the trend indicators entirely.

## Migration

No action required.
