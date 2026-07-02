## Summary

Insights and Traces now present Slack-only users with clearer labels and tooltips. Slack directory sync stores Slack real names as the primary observed label when Slack provides them, and Slack-only rows consistently explain themselves with the short "Slack User" tooltip.

## Design

The Slack OAuth directory sync uses Slack `real_name` before display name or username when persisting observed profile labels. Insights then trusts that server-provided Slack display label, while client-side trace rendering mirrors the same stale-handle fallback for rows that only have Slack handles.

Slack-only user rows keep their Slack deep links and use the tooltip "Slack User". Avatars in the agents table's used-by column show the person's name on hover instead of repeating the Slack-only tooltip, so compact avatar stacks remain identifiable.

## Migration

No migration is required. Existing observed Slack rows keep their stored labels until Slack directory sync refreshes them.
