# Fix dark mode page backgrounds

## Summary

Several listing pages (Agents, Blueprints, Explore, Knowledge Stores) had their
`PageContainer` background overridden to `bg-stone-100` without a corresponding
dark-mode class, causing them to render with a white background in dark mode
instead of the expected dark teal.

## Design

Added `dark:bg-muted` alongside the existing `bg-stone-100` override on each
affected `PageContainer`. In dark mode `bg-muted` resolves to the theme's
`--color-teal-800`, restoring the standard dark background while keeping the
lighter `stone-100` for light mode.

## Migration

No migration required.
