## Summary

Removes the BETA label baked into the app logo SVGs and moves beta messaging to a less prominent location in the settings layout.

## Design

The BETA text was embedded directly in the SVG asset, making it hard to remove independently of the logo. It's been stripped from both `astro-logo.svg` and `astro-logo-dark.svg`. The logo is also replaced with the new PNG assets (light/dark variants) and resized to `h-4` to match the marketing site.

A single line — "Astro AI is currently in beta." — now appears centered at the bottom of the settings page, outside the constrained content container so it spans the full viewport width.

## Migration

No action required.
