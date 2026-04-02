## Summary

Extracts two generic display primitives into the shared component library, migrates existing usages, and adds Storybook stories.

## Design

**`MultiSelect` (`components/ui/multi-select.tsx`)** — A composable, compound-component multi-select built on Radix UI's Popover. Consumers assemble it from primitives: `MultiSelect` (root with context), `MultiSelectTrigger`, `MultiSelectValue`, `MultiSelectContent`, `MultiSelectAllItem`, and `MultiSelectItem`. Items optionally accept a `color` prop for a dot indicator. The trigger mirrors the existing `input` base styles and applies focus/active ring states consistently.

`MonitorTab` previously used a legacy inline-styled `MultiSelect` from `components/deployed-agent/detail/shared/`; it now uses the new primitive.

**`MetricCard` (`components/MetricCard.tsx`)** — A card that displays a labeled metric value with an optional week-over-week trend indicator. The trend arrow and color (green / coral) are driven by `higherIsBetter`, so the same component works for metrics where up is good (requests) and where up is bad (error rate). Both value and trend support independent loading skeleton states.

Storybook stories and unit tests are included for both components.

## Migration

No changes required for consumers of existing components — the visual output is identical.
