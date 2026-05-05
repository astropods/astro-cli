## Summary

The create-blueprint wizard had several raw color values bypassing the design token system: hex literals, palette utilities (`text-green-700`, `text-amber-600`, `border-stone-300`, `bg-white`), a page gradient that didn't theme, and inline `white` overlays in the avatar scan effect. This change brings the wizard in line with the `local-theme/no-raw-theme-colors` ESLint rule and gives the page a proper dark-mode appearance.

## Design

**Status colors → semantic tokens.** Green checkmarks (GitHub-connected indicator, selected repo) now use `text-success`. Amber name-validation messages use the project's documented `text-yellow-700 dark:text-yellow-400` pattern (no semantic warning token exists; raw palette with explicit `dark:` variant is the sanctioned escape hatch).

**GitHub brand chip → integration icon system.** The `LinkConfirmDialog` previously rendered a hand-rolled dark circle (`bg-[#1b1f23]`) with a white `<GitHubIcon>`. It now uses `getIntegrationIcon("github")` on a `bg-card` chip, matching how every other GitHub mark in the app renders (dark asset in light mode, white asset in dark mode).

**Page background gradient.** Moved the inline `style={{ background: ... }}` into a `.dp-blueprint-bg` utility class in `index.css`, with a `.dark .dp-blueprint-bg` override. Light mode keeps the existing cream/teal radial. Dark mode uses a softer indigo halo at top + teal accent at bottom-right via `color-mix(in oklch, var(--color-indigo-600) 22%, transparent)` over `var(--background)`.

**Avatar scan effect.** The "blueprint registered" review step has a scanning overlay animation. The four corner brackets, the animated horizontal line, and the inline `@keyframes scanLine` were raw `white` / `rgba(255,255,255,...)` literals. Consolidated into `.dp-scan-line` and `.dp-scan-corner` utility classes alongside the renamed `@keyframes dp-scan-line`. The `mix-blend-overlay` keeps the white luminous in both modes — no theme variant needed.

**Card chrome + dashed border.** The wizard step card switched from `bg-white` → `bg-card` (themes correctly). The "Up next" dashed circle moved from `border-stone-300` → `border-border`.

**Selection state contrast.** Indigo selection affordances read poorly against `bg-card` in dark mode because dark `--primary` (indigo-600) sits close in luminance to `bg-card` (slate-800). Bumped the source-path and visibility option cards from `border-primary/50 ring-primary/20` → `border-primary ring-primary/40`, and the repo/branch picker selected rows from `bg-primary/5` → `bg-primary/15`. The wider issue (dark-mode primary contrast) likely warrants a theme-level adjustment in a follow-up.

## Migration

No action required. Visual-only change; no API or behavior changes.
