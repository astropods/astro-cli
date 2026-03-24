# Fix: Deploy Agent Card Stats

## Summary

The "Requests" and "Last active" stats on deployed agent cards in the `/agents` page were always showing `0` and `—` respectively, hardcoded in `YourAgents.tsx`. This change wires them up to live observability data.

## Design

A new `AgentCardWithStats` wrapper component is introduced in `YourAgents.tsx`. For each card it calls two existing query hooks:

- `useObservabilitySummary(deploymentId)` — provides `total_traces` as the **Requests** count (all-time, no time filter)
- `useObservabilityTraces(deploymentId, { limit: "1" })` — fetches the single most recent trace to derive **Last active**

Last active is formatted as a human-readable relative string ("5 minutes ago", "3 days ago") using `Intl.RelativeTimeFormat`. While data is loading it shows `—`; once loaded with no traces it shows `Never`.

Both queries are cached by React Query (5-minute stale time), so navigating back to the page won't re-fetch unnecessarily.

## Migration

No action required.
