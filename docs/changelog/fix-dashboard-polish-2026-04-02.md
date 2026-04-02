## Summary

Two polish fixes to the agent dashboard: back navigation now preserves org account query params, and the skeleton loading state uses the real deployment count instead of a hardcoded four.

## Design

**Back button preserves org account**

Clicking "View deployment" from the deploy reveal overlay passed `{ fromAgents: true }` in router state, causing `DeployedAgentDetail` to use the hardcoded `dashboardPath` (`/dashboard`) as the back destination — dropping `?account=my-org` for org accounts. The fix stores the full source path as `backPath` in state alongside `fromAgents`. `DeployedAgentDetail` reads `backPath` when present and falls back to `dashboardPath` for navigations that don't carry it.

**Server-side skeleton count**

`DeployedAgentsSection` previously rendered a fixed four `AgentCardSkeleton` elements during loading. The dashboard loader now pre-fetches the deployment count for the active account — using the `?account=` query param when present, falling back to the personal account — and passes it down as a `skeletonCount` prop. This means the skeleton grid matches the real agent count on first load.

## Migration

No action required.
