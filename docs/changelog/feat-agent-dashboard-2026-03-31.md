# Agent Dashboard

## Summary

Adds a new `/dashboard` route as a richer replacement for the `/agents` ("My Agents") page. The existing My Agents page is preserved and remains the authoritative agent list; the dashboard is introduced as a net-new parallel experience to be iterated on. No existing pages or components are broken.

## Design

The dashboard is a parent layout (`AgentDashboard.tsx`) that composes small, focused child components:

- **`DashboardHeader`** — filter input and grid/list view toggle (`ToggleGroup`) alongside "Browse blueprints".
- **`DashboardAgentCard`** — wraps `DeployedAgentCard` with per-card observability fetches (`useObservabilitySummary`, `useObservabilityTraces`) for request counts and last-active time. Clicking a card opens the preview panel instead of navigating.
- **`DashboardAgentCardSkeleton`** — loading placeholder matching the card dimensions.
- **`AgentPreviewPanel`** — a 420px inline side panel (non-overlay; the agent grid shrinks alongside it) showing identity, overview stats, and a performance section. Includes prev/next navigation between filtered agents.
- **`AgentPerformanceSection`** — owns the `useObservabilitySummary` call; renders avg latency, p95 latency, error rate, and total traces. Shows skeletons while loading and a neutral empty state when no data exists yet.
- **`AgentStatItem`** — small reusable labeled metric display used across the panel sections.

`formatRelativeTime` was extracted from its local duplicates in `YourAgents.tsx` and `DeployedAgentCard.tsx` into the shared `lib/deployment-utils.ts`, alongside the existing `formatDate`. `DeployedAgentCard` now also accepts additive optional props: `onClick`, `isPinned`/`onPin`, and `installedAtLabel`/`updatedAtLabel`.

## Migration

No action required. Existing routes, the My Agents page, and all pages using `DeployedAgentCard` are unaffected. The new `onClick` and pin props on `DeployedAgentCard` are optional with no defaults that change existing behavior.
