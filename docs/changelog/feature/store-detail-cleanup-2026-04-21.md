# Knowledge Store Detail — Comprehensive UI Polish

## Summary

A full design pass on the Knowledge Store detail page, bringing it up to the visual quality bar established by the post-create screen and the private link step card from PR #753. The page had accumulated several inconsistencies across tabs, empty states, and display modes that this cleans up.

## Design

**Settings tab** restructured around a card-based two-column row layout (`SettingRow` helper) matching the Render-style inspo — labels in a fixed 220px left column, values in the right. Three distinct cards: Configuration (managed-only), Credentials, and Danger Zone.

- Configuration card renders Storage as plain text and Public Access as a disabled Switch + hostname, rather than disabled inputs — more appropriate for read-only display.
- Credentials card expands/collapses behind a "View credentials" button in the card header. Each credential shows a description where the field name isn't self-explanatory. Inputs are `readOnly` (not `disabled`) to avoid the opacity side effect, with `focus-visible:ring-0` to suppress focus rings.
- External stores omit Configuration (nothing to show) and collapse to full-width agent bindings with no event log sidebar.
- Danger Zone uses the same border style as other cards (no separate destructive border color).

**Overview tab** improvements:
- Pending acceptance state now hides metrics and agent bindings — only the `PrivateLinkSection` step card is shown.
- Agent count displayed as a `Tag` component next to the section heading.
- Bot icon used in the empty state for agent bindings (matching the header icon).
- Event log card header padding tightened.

**PrivateLinkSection** rebuilt to match the PR #753 step card pattern — `size-8` numbered circles, `stone-100` pill badges for endpoint ID and region, cloud console CTA only when pending, and a warning banner above the card rather than inside it.

**MetricCard** extended with optional `sparkline`, `valueSuffix`, and `description` props. Sparklines use a teal gradient `AreaChart` with a 1s ease-out animation.

**LogViewer** scroll area background changed to `bg-white` for consistency with other content areas.

Minor label fixes: `"Pending Acceptance"` → `"Pending"`, `"Online"` → `"Ready"` in status labels.

## Migration

No API or data model changes required. The `bound_agents` and `uptime_seconds` fields on `KnowledgeStore` are additive optional fields.
