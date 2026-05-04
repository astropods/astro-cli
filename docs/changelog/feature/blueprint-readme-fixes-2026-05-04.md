## Summary

Polish on the blueprint detail page readme section and supporting primitives so the rendered markdown, integration badges, and brand icons look correct in both light and dark themes.

## Design

- Renamed the readme section label from "ReadMe" to "AGENT.md" to mirror the source filename agents reference when authoring blueprints.
- `StyledMarkdown` swapped raw `bg-teal-900`, `bg-stone-200`, and `text-code-text` palette utilities for semantic tokens (`bg-muted`, `border-border`, `text-foreground`) on code blocks, inline code, and table headers — these previously failed to flip cleanly across themes. Code block radius now matches the surrounding readme card at `4px`, and link styling moves from brand `text-primary` to `text-foreground` with a softer underline.
- `getIntegrationIcon` now renders both light and dark CDN variants and toggles them via the `dark:` class so single-color brand marks (e.g. GitHub) remain visible on dark surfaces.
- `InlineBadge` default variant drops the `bg-stone-200` / `dark:bg-teal-900/40` fill and teal border/text, becoming a clean `text-foreground` + `border-border-strong` chip. The `fill` variant moves to `bg-accent`. `RequiredAppsList` no longer needs per-mode color overrides as a result.

## Migration

No changes required.
