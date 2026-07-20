# Fix chat agent switcher missing a chattable agent until refresh

## Summary

On the full-page chat, the agent switcher sometimes omitted an agent the user
could actually chat with — even one they were actively chatting with — until a
hard page refresh. It showed up after deploying or activating an agent.

## Design

The switcher draws from two different queries with different freshness:

- The switch list's **candidates** come from the deployments summary
  (`useDeploymentsSummary`) — `staleTime` 60s, no polling, and invalidated only
  on undeploy.
- **Eligibility** (`eligibleDeploymentIds`) comes from the per-account
  deployment lists in `useChatAgents` — `staleTime` 30s and polled every few
  seconds while a deployment is transitional.

`AgentDeploymentMenu` renders `summary ∩ eligible`, so an agent that is eligible
(present in the fresh per-account list) but not yet in the stale summary is
dropped from the menu. Deploying an agent invalidates the per-account list but
not the summary, so the per-account list "sees" the new agent within seconds
while the summary doesn't — hence the missing entry until a refresh reloads the
summary. (The summary payload can't even express eligibility on its own — its
item shape omits `messaging_web_configured` — so single-sourcing eligibility off
it isn't possible.)

The fix reconciles the two sources in `useChatAgents`: when the fresh per-account
lists surface a chat-eligible agent that isn't in the summary yet, it invalidates
the summary so it refetches (the server returns the same rows as the per-account
list, so the refetch includes the agent). It's keyed on sorted id-strings of the
two sets, so a refetch that returns the same set doesn't re-trigger — no
invalidation loop, and it self-heals without a manual refresh.

## Migration

None.
