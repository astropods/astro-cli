# Storage-usage: event-driven instead of polled

## Summary

The storage-capacity banner drove its reading off a fixed 60s poll of
`/deployments/:id/files/usage`. For any deployment that isn't running, that route
5xxes — the messaging Service exists but has no ready pod, so the K8s apiserver
proxy returns 503 and astro-server forwarded it verbatim. A single user sitting
on the chat/monitor page of a stopped deployment produced a steady ~1 5xx/min,
which on a low-traffic route trips `AstroServerHigh5xxRateByRoute`. (This is a
distinct cause from the earlier no-web-adapter fix, which only covered the
resolve-time "no http port" failure.)

## Design

Usage only changes when files change, so a wall-clock timer is the wrong trigger.

**Client — event-driven, no timer.** `useDeploymentStorageUsage` drops
`refetchInterval`. The reading is fetched on mount and refreshed by cache
invalidation at the moments files can actually change:
- file upload/delete success (mutation `onSuccess`),
- chat-turn finish (`use-deployment-chat` `finalizeConversation`) — the agent may
  write files during a turn; this event replaces the old poll for agent-driven growth.

The banner also gates the query on run-state via `useDeploymentStatus`
(`enabled` only when `value === "active"`), so a stopped deployment is never
fetched. Tradeoff: file growth outside an observed chat turn (e.g. a background
agent while nobody's on the page) isn't reflected until the page remounts —
acceptable for a soft capacity warning.

**Server — defense-in-depth guard.** `forwardFiles` now returns **404** when the
deployment's DB status isn't `active`, before dialing the sidecar. This reserves
5xx for genuine faults and keeps the alert quiet even if some other caller hits
the route on a not-running deployment. Genuine upstream failures on active
deployments still surface as 5xx.

## Migration

None. No API contract or configuration changes for users.
