---
PR: feature/knowledge-store-design-cleanup
Date: 2026-05-14
---

## Summary

Visual polish pass on the knowledge store creation flow — dark mode correctness, semantic color token consistency, mobile responsiveness, and restoring the success state that was dropped in a prior refactor.

## Design

**Theme correctness**
- Provider cards and all post-create store cards use `bg-card`/`bg-surface` elevation tokens instead of raw `bg-white`. Hover states use `bg-muted` rather than a border color change.
- Radio option cards in the configure form use `bg-muted` for hover and icon backgrounds (was raw `slate-200/50`).
- Dark mode input background changed from `slate-800` → `slate-950` in the theme semantic tokens, giving inputs a recessed feel rather than appearing elevated/disabled.
- Outline button changed from `bg-background` → `bg-transparent` so it inherits its container's background rather than always showing the page background color.

**Provider icons**
- `ProviderIcon` now reads `useTheme()` and passes the correct `"light"` or `"dark"` variant to `getIntegrationIconUrl`, which already had both asset sets.
- Added `dark:brightness-150` to boost low-contrast dark variants (e.g. MySQL's `#00678c` fill disappears on dark backgrounds).

**Semantic color tokens**
- All hardcoded `teal-*` and `yellow-*` colors replaced with `--success` and `--warning` semantic tokens across `ProvisioningStage`, `PendingAcceptanceStage`, and `PrivateLinkSection`.
- Provisioning step checkmarks and spinner: `bg-success dark:bg-green-600` (green-600 pinned in dark mode to maintain white text contrast — `green-400`, the dark mode success token, is too light for white text).
- PrivateLinkSection step circles use `border-success/30 bg-success/10` and `border-warning/30 bg-warning/10` to match the same pattern.
- PrivateLinkSection steps card background aligned to `dark:bg-surface` to match the store card above it.

**Success state**
- Restores the polished success screen dropped in the `NewKnowledgeStore` refactor: animated check icon (`ks-pop` + `ks-check-draw` keyframes), full-page confetti, store card with provider icon + Ready badge, and stacked CTAs.
- Check icon and ring use `--success` semantic token.
- `handleProvisionReady` dispatches to a `"success"` step instead of navigating away immediately.

**PendingAcceptanceStage stepper**
- Step labels changed from colored (`text-success`, `text-warning`) to `text-foreground` for active steps and `text-faint-foreground` for the locked "Connected" step.
- Buttons aligned to match success state: stacked `flex-col-reverse`, ghost below primary.

**Mobile**
- Provisioning and success store cards stack the badge row below the name/icon at narrow widths (`flex-col sm:flex-row`).
- Success CTA buttons always stack vertically, primary on top.

## Migration

No migration required.
