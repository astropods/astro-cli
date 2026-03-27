## Summary

Adds a trace detail panel to the Monitor tab with rich markdown rendering for input/output, replaces the tabbed layout with stacked collapsible accordions, and standardises "Show more" buttons across the detail page.

## Design

### Trace detail panel (V2 — accordion layout)

A slide-out right panel (420px, sticky) for inspecting individual traces, reusing the Configure panel pattern. V2 replaces the original tabbed input/output view with two stacked collapsible accordion sections so both are visible simultaneously. Each section has an icon (↗ Input, ↙ Output), a per-section copy button, and smooth expand/collapse animation via CSS grid rows. Typography uses design system tokens (`text-body`, `text-body-sm`, `text-label`) throughout.

On compact viewports the panel renders full-page, matching Configure's compact behaviour.

### Rich text rendering

Input and output render via `StyledMarkdown` instead of raw strings, supporting headings, lists, tables, code blocks, and inline code.

### Button consistency

"See X more" / "See less" renamed to "Show X more" / "Show less" on both the traces list and deployment history table. The deployment history button was converted from a raw `<button>` with monospace styling to a ghost `Button` with chevron icons to match the traces pattern. Service empty state text switched from mono to sans.

## Migration

No migration required.
