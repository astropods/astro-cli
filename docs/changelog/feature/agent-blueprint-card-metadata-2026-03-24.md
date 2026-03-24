# Blueprint Card Metadata Updates

## Summary

Updates the metadata displayed on agent blueprint cards to surface deploy count and account identity instead of lifetime message count.

## Design

- **Deploy count** replaces lifetime message count in the default card footer, displayed as "X deploys".
- **Account avatar** (16px, round) added to the left of the account name in the default card footer using the existing `UserAvatar` component.
- **oftenUsedTogether variant** updated to show deploy count and account name separated by a bullet divider (`•`).
- **`deploy_count`** added as an optional field to the `AgentMetrics` type in `api.ts`.

## Migration

No migration required. `deployCount` is optional; cards render "0 deploys" when not provided.
