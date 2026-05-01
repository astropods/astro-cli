# Color system overhaul: slate neutral palette + indigo primary

## Summary

Replaces the stone neutral palette and teal primary color with a coherent slate + indigo system. All semantic tokens, component fills, and raw palette overrides are updated so the UI adapts correctly across light and dark mode without hardcoded values.

## Design

**Palette changes**
- New `slate` scale in OKLCH (12 steps, perceptually uniform) replaces `stone` as the neutral
- `teal` restored to proper saturation for use as a semantic accent (not the primary)
- `primary` switched to indigo-700 (light) / indigo-600 (dark)
- New `success` semantic token (green-600 / green-400) for copy checkmarks, uptime indicators, StatusBadge success variant, and Tag teal variant

**Semantic token updates**
- All background/surface/card/border tokens now reference `slate-*`
- All foreground text tokens migrated from `stone-*` to `slate-*`
- `card` and `popover` use `#fff` in light mode for clean white surfaces
- `primary-*` Tailwind scale updated from teal to indigo throughout `@theme inline`

**Component-level fixes**
- `StatusBadge` success: hardcoded `rgba(teal)` → `color-mix(var(--success))`
- `Tag`: fg split into Tailwind class with proper `dark:` variants per hue
- `DeployedAgentCard`: hover border uses `primary/40` (indigo), not slate (which read as teal)
- `MetricCard` / `ComputeUsageCard`: `dark:bg-surface` instead of raw `dark:bg-slate-900`
- `BlueprintCard` draft: white fill in light mode, legible dark in dark mode
- Section headers inside cards (`bg-surface`) now correctly differentiate from card body in both modes
- Blueprint setup flow, breadcrumb, OrgSwitcher, nav shell all use semantic tokens

## Migration

No user-facing changes or API changes. Design tokens in `@astropods/theme` are source-of-truth; consuming apps get updates automatically on next build.
