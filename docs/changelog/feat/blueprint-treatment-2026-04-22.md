# Blueprint card visual treatment + server-provided avatar colors integration

## Summary

Redesigns the BlueprintCard with a blueprint-inspired visual treatment (grid overlay, fold lines, tinted background derived from the avatar's dominant color) and replaces all client-side color extraction with server-provided `avatar_colors` from the API.

## Design

### Blueprint card treatment

The default BlueprintCard variant now renders a layered visual treatment:

- **Tinted background** — `color-mix(in srgb, <accent> var(--mix), var(--card-neutral))` blends the avatar's dominant color with a teal-tinted neutral. A CSS custom property `--mix` toggles between 18% (default) and 14% (hover) for a subtle lighten-on-hover effect.
- **Grid overlay** — A `::before` pseudo-element renders a repeating 8px CSS gradient grid.
- **Fold lines** — 1–2 vertical fold shadows with seeded random placement and rotation, creating the appearance of a folded blueprint.
- **Diagonal fade** — A white-to-transparent gradient that masks the bottom-right for depth.
- **Inset border** — A `::after` pseudo-element at `inset: 3px` with a 2px teal-25 border.
- **Dark mode** — All treatments adapt via CSS custom properties (`--card-neutral`, `--card-contrast`, `--card-grid`).

### Server-provided color integration

All three systems that previously extracted colors client-side now consume `avatar_colors` from the API:

1. **BlueprintCard** — `avatarColors` prop replaces canvas-based extraction. The `extractAccentFromImg`, `rgbToHsl`, and `hslToHex` utility functions are removed.
2. **LiveRevealOverlay** — `useCardColors(deployment.avatar_colors)` replaces `useExtractedColors`. Colors from the blueprint are passed through navigation state during deploy, with the upload response providing fresh colors when a custom avatar is staged.
3. **TradingCardModal** (share badge) — Accepts `avatarColors` prop, maps to `CardColors` via `useCardColors`. Wired through `KebabMenu` (detail page) and `DeployedAgentCard` (dashboard).

The `extractColorsFromImage` and `svgToImageSource` functions are removed from `astro-trading-card/browser` exports. The `extract-colors.ts` module is deleted entirely.

### Backend changes

- `AvatarResponse` now includes `avatar_colors` alongside `avatar_url` for all avatar upload/preset/reset endpoints.
- `enrichDeployment.applyDBFields` now copies `avatar_colors` from the DB record onto K8s-enriched deployment responses.
- Backfill workers always log completion stats (even when all counters are zero) for production observability.

## Migration

No manual migration required. The `avatar_colors` column was added in the previous PR. The backfill workers populate colors for existing entities automatically. Frontend changes are backward-compatible — components gracefully fall back to default colors when `avatar_colors` is absent.
