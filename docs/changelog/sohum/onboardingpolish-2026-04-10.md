## Summary

Polish pass on the blueprint creation and draft card UI to reduce visual noise and improve consistency across the onboarding flow.

## Design

**Draft blueprint card** — the card now uses a dashed `border-stone-400` border (same color as normal cards) instead of yellow, has no background fill, and shows a dashed inner divider above the deploys/creator row. The "Finish setup" pill uses the design system's soft yellow variant (`variant="soft"` with `color-mix`) with no border.

**Blueprint detail setup section** — "View examples" and "View package spec" links moved into the section header bar alongside the "Finish setup" label. Command blocks use `bg-white` fill and plain `text-foreground` throughout (syntax highlighting removed).

**Blueprint creation carousel** — restructured from absolute-positioned fixed-height panels (`h-[540px]`) to a flex-row sliding layout with `min-h-[460px]`, so height is content-driven and all panels stay consistent. Bottom padding on the setup step content removed to eliminate the gap between the visibility picker and the footer button.

**Empty state mascots** — trio size reduced from 48→36 and reordered to Gear | Square | Star (square centered).

**Copy** — "Finish setting up" → "Finish setup" across card, detail header, and section bar. "Up next" copy updated to "Set up your agent in code".

## Migration

No changes required.
