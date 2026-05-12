## Summary

The docs site background colors were hardcoded hex values that didn't match the Astro design system's `--background` token, causing a visual mismatch between docs and the rest of the product (especially noticeable in dark mode, where the old value was a blue-navy `#01031a` vs. the teal-tinted dark background the design system uses).

## Design

Fern's `docs.yml` accepts only hex values for its color theming. The `--background` token from `packages/astro-theme` is defined in OKLCH — `oklch(14.89% 0.0080 164.578)` for dark and `oklch(97.89% 0.0068 77.488)` (stone-100) for light. These were converted to their sRGB hex equivalents and applied to the `colors.background` block in `docs-public/fern/docs.yml`.

## Migration

No action required.
