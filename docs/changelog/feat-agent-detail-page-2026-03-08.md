# Redesign agent detail page with new tokens and StyledMarkdown

## Summary

The agent detail page was overhauled with refined typography, tighter spacing, and a new `StyledMarkdown` component for rich README rendering. Two new semantic tokens (`border-strong`, `code-text`) were added to `astro-theme` to replace hardcoded one-off values with proper light/dark mode support.

## Design

- **`StyledMarkdown`** — Reusable component wrapping `react-markdown` + `remark-gfm` with comprehensive prose styling for headings, tables, code blocks, task lists, and inline code. Replaces the inline prose class blob that was in `AgentDetailContent`.
- **`border-strong` token** — A heavier border tier (`stone-400` light, `teal-300/25%` dark) used for dividers, card borders, and section separators across the detail page. Eliminates the repeated `border-stone-400 dark:border-border` pattern.
- **`code-text` token** — Semantic color for code block text (`oklch(82.90% 0.0224 182.6)` light, `teal-300` dark). Replaces a raw hex value.
- **Layout** — Responsive breakpoint changed from `lg` to `min-[900px]` for better mid-size screen support. Header uses larger agent identity with mono typography. Breadcrumb bar uses the `surface` background with mono type.
- **Shared updates** — `InlineBadge` restyled as rounded pills, button hover states lightened, body background switched to `bg-surface`.

## Migration

No migration required.
