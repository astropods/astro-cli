# Polish: Deployed Agent Card — Update Available Badge

## Summary

The "update" badge on the deployed agent card was a plain teal `InlineBadge`. This replaces it with an outlined tag in the warning yellow palette, matching the blueprint detail tag style, with an `ArrowUpCircleIcon` and clearer copy.

## Design

The `InlineBadge` for `hasNewBuildAvailable` in `DeployedAgentCard` is updated to use:
- Geist sans-serif at 12px (instead of mono uppercase) for a softer, readable feel
- Outlined style (transparent fill, subtle yellow border) so it reads as secondary to the status badge
- `ArrowUpCircleIcon` from Heroicons to reinforce the action meaning
- Copy changed from "update" to "Update available"

A `WithUpdateAvailable` Storybook story is added under `Features/Agents/DeployedAgentCard` to document the variant.

## Migration

No action required.
