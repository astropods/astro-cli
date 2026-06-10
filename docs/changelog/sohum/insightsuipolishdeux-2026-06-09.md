## Summary

Insights table polish makes the ranking column feel less detached from identity rows, softens unlinked Slack users, and keeps long lists responsive.

## Design

Rank now lives inside the identity cell for both People and Agents, so row order and identity scan as one unit. Unlinked Slack users use the same quiet anonymous-user treatment across the People table and the Agents Used By stack, with shared label formatting to avoid divergent render paths.

Agent chips in the People table's Agents Used column use the BlueprintCard avatar treatment: 24px square icons, 3px corners, a subtle border, and matching focus-ring radius.

Long tables reveal additional rows progressively instead of mounting every hidden row in one click. The first five rows remain the default viewport, the first reveal shows the top ten, and the next reveal shows the full filtered set. Row animation uses a short capped stagger so expanding or collapsing large result sets stays quick.

Search continues to run against the full unfiltered dataset before the visible-row window is applied, so hidden rows are still discoverable.

## Migration

No migration required.
