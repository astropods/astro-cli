# Unify CLI info box styles

## Summary

The `astro dev` ready block used a double border with the theme's primary color (cyan/magenta), while `astro create` used a rounded border in teal with a green/yellow/blue interior palette. This unifies `astro dev` to match.

## Design

Replaced `printReadyBlock` in `cmd/dev.go` to use the same lipgloss style tokens as `printSuccess` in `cmd/create.go`: rounded border in teal (`"62"`), green headings (`"10"`), yellow commands (`"11"`), blue accents (`"12"`), and dimmed descriptions. Removed the now-unused `theme` import from `dev.go`.

## Migration

No migration required.
