# Trading Card Design Tweaks & QR Code

## Summary

Refinements to the trading card rendering: improved typography and layout for stat rows, smarter text handling for usernames, bounded integration badges, and a QR code beside the barcode.

## Design

**Stat row typography** — Label font size increased from 7px to 9px. Account handle text now matches the regular value style (13px/700 weight) and the `@` prefix is removed for a cleaner look.

**Canvas-based text measurement** — Handle text width is now measured via a `<canvas>` context at render time (with a character-width fallback for non-browser environments). This replaces the previous `charCount * 0.55` approximation so the avatar circle is positioned accurately next to the handle. Long handles are truncated with an ellipsis based on the available width after accounting for the label on the left.

**Integration badge overflow** — Badges are capped at 2 rows. When there are more integrations than fit, a `+N` label is appended after the last visible badge.

**QR code** — A new optional `qrUrl` field on `CardData` encodes a URL as a QR code rendered to the right of the barcode. The barcode shrinks horizontally to make room. QR encoding uses the `uqr` library (~3 KB, zero deps) which returns a boolean matrix that gets rendered as SVG rects.

**Dev client scenarios** — The dev client now supports per-sample card data overrides, with dedicated test cases for many integrations (overflow), no integrations, no stats, long handles, and very long handles (truncation).

## Migration

No breaking changes. `qrUrl` is optional — existing cards render identically without it.
