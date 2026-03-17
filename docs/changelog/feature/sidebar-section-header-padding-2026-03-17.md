## Summary

Visual polish pass on the AgentDetailSidebar and AgentCard components — tightening spacing, updating copy, and aligning iconography with the "deploy" terminology.

## Design

- Reduced sidebar section header padding from `py-2.5` to `py-2` for consistency with the Details panel.
- Removed `@` prefix from account label in `oftenUsedTogether` card; styled to match default card (`text-faint-foreground`).
- Renamed "Installs" → "Deployments" and "Rating" → "Requests" in the Details panel.
- Replaced `ArrowDownTrayIcon` with `RocketLaunchIcon` next to Deployments count.
- Removed star icon and download icon from both card variants.
- Simplified `oftenUsedTogether` card to show only agent name and account — no metrics.
- Set `oftenUsedTogether` card padding to `px-3 py-2` and aligned content with `items-center`.

## Migration

No changes required.
