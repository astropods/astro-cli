# Perceptual lightness normalization for blueprint cards in light mode

## Summary

Blueprint cards derive their background tint and muted text color from the avatar's extracted accent color. In light mode, perceptually bright accents (e.g. light blues) produced cards that looked washed out, while darker accents appeared fine. A flat increase to the color-mix ratio would over-darken already-dark accents, so the fix needed to scale proportionally with perceived brightness.

## Design

Two complementary adjustments are applied per card in light mode only (dark mode is unchanged):

1. **oklch perceptual darkening** — The accent base color is darkened by mixing with black in oklch color space before it enters the card background formula. The amount of darkening scales with sRGB relative luminance: bright accents get up to 20% black mixed in, while dark accents pass through nearly untouched. oklch preserves hue and distributes lightness perceptually, so colors stay clean rather than muddy.

2. **Adaptive mix percentage** — The proportion of (now-darkened) accent mixed into the card neutral also scales with luminance: 26% for dark accents up to 42% for light ones. This amplifies the tint for colors that need it without over-saturating those that don't. Hover states scale proportionally (80% of base mix).

Both the card background and the muted footer text color use the darkened accent, keeping them visually consistent.

## Migration

No migration required — this is a purely visual change to light-mode blueprint card rendering.
