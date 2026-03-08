# Redesign install page with sketchbook-inspired styling

## Summary

The install/deploy page was redesigned with refined typography, tighter spacing, and a visual style consistent with the recent agent detail page overhaul.

## Design

- **Header** — Replaced breadcrumb navigation with a compact header bar containing a back arrow to the agent detail page, the agent identity avatar, and the page title as an `h1`.
- **Form sections** — `FormSection` headings use the `border-strong` token for dividers. Ink color references switched from raw CSS variables (`var(--ink-faint)`) to semantic Tailwind utility classes (`text-ink-faint`, `text-ink`).
- **Interfaces picker** — Card layout and typography updated with semantic color classes and a default value for the `isBrand` flag.

## Migration

No migration required.
