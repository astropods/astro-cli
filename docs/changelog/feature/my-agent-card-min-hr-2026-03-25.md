# My Agent Card — Date/Time Improvements

## Summary

The deployed agent card on the My Agents page showed dates without time, making it hard to distinguish deployments from the same day. The "Updated" field also had no relative context for recency.

## Design

- **Deployed** field now shows full date + time (e.g. "Mar 25, 2026, 2:45 PM") using `formatDateTime` defined locally in `DeployedAgentCard`.
- **Updated** field now shows relative time (e.g. "3 hours ago", "2 days ago") using `formatRelativeTime`, also defined in `DeployedAgentCard`.
- Both fields accept raw ISO strings — formatting is handled entirely at the component level.
- The `AgentDeployment` type has no `updated_at` field yet; both fields use `created_at` as a stand-in until the backend adds it.

## Migration

No action required.
