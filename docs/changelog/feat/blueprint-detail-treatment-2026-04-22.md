# Blueprint detail page visual treatment

## Summary

Applies the new blueprint visual language to the detail page — avatar-colored gradient background with grid lines, sharpened panel edges, and a holographic deploy CTA button that responds to cursor proximity and hover.

## Design

### Page background treatment

A decorative background sits below the sticky breadcrumb at the top of the detail page scroll area. It combines two layers masked by a radial gradient so they fade naturally:

- **Color wash** — A radial gradient using the blueprint's `avatar_colors.glow` (light mode) or `avatar_colors.base` (dark mode) via `color-mix()`. Positioned at 25% from left to align with the avatar.
- **Grid lines** — An SVG `<pattern>` at 8px spacing. Light mode uses `vibrant_light` stroke at 0.35 opacity; dark mode uses white at 0.12 opacity. Separate SVGs per theme allow independent tuning.

### Panel edge sharpening

All bordered panels on the detail page (sidebar sections, code blocks, setup steps, readme) use `rounded-[4px]` instead of the previous `rounded-md` (6px), matching the tighter card aesthetic.

### HoloButton component

A new `HoloButton` (`src/components/ui/holo-button.tsx`) wraps the existing `Button` component and adds layered holographic effects derived from the avatar accent color:

- **Color derivation** — Extracts hue from the accent hex and generates HSL variants at 50% saturation with controlled lightness (45%/55% light, 32%/42% dark).
- **Noise texture** — SVG `feTurbulence` fractal noise at 30% opacity with `mix-blend-overlay`.
- **Border glow** — A 1px inset border rendered via CSS mask-composite trick (`exclude`). A radial gradient follows cursor position so only the border segment near the cursor lights up.
- **Shine layer** — The same triple-gradient `color-dodge` stack from the trading card holo effect (radial cursor-tracking gradient, diagonal shifting gradient, rainbow repeating gradient).
- **Glare layer** — A `mix-blend-overlay` specular radial gradient tracking the cursor.
- **Proximity detection** — The component uses negative margin + padding to create an invisible extended hit area. The border glow activates when the cursor enters this zone (before reaching the button), while the shine/glare layers only activate on direct hover. Two separate CSS custom properties (`--o` for proximity, `--ho` for direct hover) control the split.
- **Composability** — Supports `asChild` passthrough to the underlying `Button`, so it works with `<Link>` children. Falls back to a plain `Button` when no accent color is provided.

### Other changes

- Added `AvatarColors` interface and `avatar_colors` field to the client `Blueprint` type.
- Renamed "More agents" sidebar section to "More blueprints".

## Migration

No migration required. All changes are client-side visual enhancements.
