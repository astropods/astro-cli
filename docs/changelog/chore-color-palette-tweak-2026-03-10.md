# Stone Palette Refinement & Surface Token Fix

## Summary

The stone color palette was visually too dark and warm for the intended parchment aesthetic. Additionally, the `surface` semantic token was hardcoded to a raw OKLCH value instead of referencing the palette, meaning palette changes had no effect on the page and nav backgrounds.

## Design

**Stone palette adjustment** — All 12 stone steps were updated in `astro-theme/src/colors.ts` using two mathematically consistent OKLCH operations: a uniform +5% lightness offset across all steps (capped at 99.75% for the lightest), and a proportional chroma reduction to ~60% of the original values. Hue values are unchanged. Chroma was reduced to move the palette from yellow-warm toward a more neutral parchment tone. Source hex anchors in `scripts/convert.ts` were reverse-calculated from the new OKLCH values using the existing pipeline, keeping the full conversion chain consistent.

**Surface token** — `--surface` in `semantic.ts` was previously set to a hardcoded `oklch(...)` value that approximated `stone-100` but was disconnected from the palette. It now references `var(--color-stone-100)` directly, so future palette changes propagate automatically.

**Nav background** — `AppHeader` was using the raw palette token `bg-stone-100` instead of the semantic `bg-surface`. Swapped to `bg-surface` so the nav moves with the surface token.

## Migration

No action required. CSS is generated at build time from `astro-theme`.
