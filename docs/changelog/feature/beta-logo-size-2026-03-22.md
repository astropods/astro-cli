# Beta logo size

## Summary
Increase the rendered height of the Astro logo in the app header to match the docs site, making the BETA tag legible and consistent across surfaces.

## Design
The BETA label is embedded in the logo SVG. Rendering the SVG taller (24px, matching `docs-public`) scales the text up proportionally. Changed `h-4` → `h-6` on both light and dark logo `<img>` elements in `AppHeader.tsx`.

## Migration
None required.
