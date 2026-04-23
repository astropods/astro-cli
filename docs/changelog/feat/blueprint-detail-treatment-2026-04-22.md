# Blueprint detail page visual treatment + target-based color extraction

## Summary

Applies the new blueprint visual language to the detail page — avatar-colored gradient background with grid lines, sharpened panel edges, and a holographic deploy CTA button. Also replaces the color extraction algorithm across both the TypeScript client and Go server with Android Palette-style target matching, and adds a one-time River migration to re-extract all existing avatar palettes.

## Design

### Page background treatment

A decorative background sits below the sticky breadcrumb at the top of the detail page scroll area. Two layers are masked by a radial gradient so they fade naturally:

- **Color wash** — A radial gradient using `avatar_colors.glow` (light) or `avatar_colors.base` (dark) via `color-mix()`, positioned at 25% from left to align with the avatar.
- **Grid lines** — An SVG `<pattern>` at 8px spacing. Light mode uses `vibrant_light` at 0.35 opacity; dark mode uses white at 0.12 opacity.

### HoloButton component

`HoloButton` (`src/components/ui/holo-button.tsx`) wraps the existing `Button` and adds layered holographic effects derived from the avatar accent color:

- **Color derivation** — Extracts hue from accent, clamps saturation to 35-75%, and sets lightness to 45% (light) / 32% (dark).
- **Noise texture** — SVG `feTurbulence` at 30% opacity with `mix-blend-overlay`.
- **Border glow** — A 1px inset border via CSS `mask-composite: exclude`. A radial gradient follows cursor position.
- **Shine + glare** — The trading card's `color-dodge` triple-gradient stack and `mix-blend-overlay` specular highlight, both tracking cursor position.
- **Proximity detection** — An absolutely-positioned invisible overlay extends the hit area beyond the button. Border glow activates on proximity (`--o`), shine/glare on direct hover (`--ho`).
- **Composability** — Supports `asChild`, falls back to plain `Button` when no accent color is provided.

### Target-based palette selection

Replaces the old `saturation × sqrt(population)` single-accent scoring with Android Palette-style target matching in both `astro-trading-card/colors.ts` and `astro-server/internal/colorextract/colorextract.go`:

- Four target profiles (vibrant, light vibrant, dark vibrant, muted), each with target/min/max bounds for saturation and lightness.
- Each role independently picks the best swatch. Scoring weights: lightness 0.52, saturation 0.24, population 0.24.
- The vibrant target's winner becomes the accent — genuinely saturated colors now win over muddier but more dominant ones.
- `accentLight` uses the light vibrant target when available.

### Color re-extraction migration

`ReextractColorsWorker` (`riverqueue/reextract_colors.go`) re-extracts `avatar_colors` for all agents and deployments using the updated algorithm. Registered as a periodic River job that runs on startup; the worker checks `river_job` for a prior completed run with its kind and no-ops if already done.

### Other changes

- Panel border radii sharpened to `rounded-[4px]` across the detail page.
- Added `AvatarColors` interface and `avatar_colors` field to the client `Blueprint` type.
- Renamed "More agents" sidebar section to "More blueprints".
- Storybook color diagnostics story with 3D HSL cylinder visualization for validating the extraction pipeline.

## Migration

The `avatar_colors.reextract_2026_04_23` River job runs automatically on first deploy. It re-processes all agent and deployment avatars, then becomes a no-op on subsequent starts. No manual action required.
