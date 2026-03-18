# Shareable agent trading cards

## Summary

Adds a new `astro-trading-card` package and integrates it into the client so users can generate and download shareable trading cards for their deployed agents. Each card features the agent's avatar, stats, integrations, and a barcode — with colors automatically extracted from the avatar and a holographic hover effect in the share modal.

## Design

The feature is split into a framework-agnostic package (`packages/astro-trading-card`) and client integration (`apps/astro-client`).

**`astro-trading-card` package** — pure SVG generation with no browser or framework dependencies in the main entry point:

- `generateCard(data)` accepts a `CardData` object and returns a self-contained SVG string. Colors are resolved against teal-themed defaults before rendering.
- Color extraction uses a Modified Median Cut Quantization (MMCQ) algorithm (`mmcq.ts`) to pull a palette from avatar pixel data, then card-specific derivation (`colors.ts`) maps that palette to background/foreground/accent/glow values.
- A separate `astro-trading-card/browser` entry point provides browser-only utilities: `extractColorsFromImage()` encapsulates the canvas→palette→colors pipeline, and `downloadSvg`/`downloadPng` handle file export with automatic external image embedding.
- The holographic effect is shared via `holo.ts` (pointer math) and `holo.css` (layered gradient/blend-mode styles), used by both the React client and the package's dev harness.

**Client integration** — a `TradingCardModal` component triggered from the deployed agent card's dropdown menu:

- Opens a dialog with the generated SVG wrapped in a `HoloCard` component for the 3D hover effect.
- Colors are extracted on modal open via the encapsulated `extractColorsFromImage()`.
- Integration icons resolve to CDN-hosted dark-variant SVGs for embedding in the card.
- SVG and PNG download buttons lazy-load the browser utilities.

**CI** — the E2E workflow and moon task graph were updated to include `astro-trading-card` as a build dependency, and a `typecheck` script was added to avoid moon's inherited task arg-merging issue.

## Migration

No changes required.
