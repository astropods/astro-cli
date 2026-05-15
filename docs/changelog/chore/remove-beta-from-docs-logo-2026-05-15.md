## Summary

Updates the docs logo to match the Astro AI website: removes the stale "BETA" label and replaces the dark-mode SVG with the PNG logo used on astropods.com.

## Design

Two changes in `docs-public/fern/docs/assets/`:

1. Removed the `<text>BETA</text>` element from both `astro-logo.svg` and `astro-logo-dark.svg`.
2. Added `astro-logo-dark.png` (sourced from the marketing website) and updated `docs.yml` to use it for dark mode. The light-mode SVG is kept as-is since the PNG uses white text.

## Migration

No action required.
