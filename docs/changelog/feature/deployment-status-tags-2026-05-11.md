---
title: Trace status indicators — StatusBadge, semantic tokens, and multi-select filter
---

## Summary

The traces surface on the deployment page used three bespoke inline-style patterns: one for the row status, one for the trace detail panel meta tile, and one for the filter chips. The colors were palette-direct (`yellow-500`, `coral-600`) and did not theme — warning and error rendered visibly duller than success in light mode and stayed locked to light-mode tokens in dark mode. The filter chips also used a custom checkbox-in-pill pattern that did not communicate "clickable" well and drifted from the multi-select dropdown the agents dashboard uses for the equivalent surface.

This change unifies all three surfaces on the existing `StatusBadge` primitive, introduces `--warning` and `--error` semantic tokens so the badge themes properly, and swaps the bespoke filter chips for the `MultiSelect` dropdown already used at `/agents`.

## Design

**Semantic tokens** — `--warning` and `--error` join `--success` as theme-aware status colors. Light maps to the `-500` step (yellow-500, coral-500) to match `--success`'s lightness band; dark maps to the `-400` step (yellow-400, coral-400) for the brighter cousins, mirroring how `--success` flips green-600 → green-400. `StatusBadge`'s warning and error variants now consume `var(--warning)` and `var(--error)` at the same 12% bg / 28% border alpha as success, so the three variants render at equal pigment density and theme together.

**Trace surfaces** — Row status (`TracesTable`) and the detail panel meta tile (`TraceMetaGrid`) render `<StatusBadge>` directly. A `STATUS_BADGE_COLOR` map in `trace-utils.ts` bridges `TraceStatus` (`success | error | timeout`) to the badge's `StatusBadgeColor` (`success | error | warning`). `STATUS_CONFIG` is reduced to just labels — the old bg/bdr/fg fields are gone now that color lives in `StatusBadge`.

**Filter** — Status filter chips are replaced by the `MultiSelect` primitive (`@/components/ui/multi-select`), matching the AgentDashboard pattern. State is `string[]` with empty-array-means-all semantics; the trigger reads "All statuses", "Success", or "N selected" via `MultiSelectValue`. Multi-select capability is preserved (any subset of statuses can be active simultaneously) inside a clearly clickable dropdown affordance.

## Migration

No action required. `--warning` and `--error` are now available as semantic tokens for any future status surface that needs theme-aware coral/yellow.
