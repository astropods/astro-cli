## Summary

Public docs were still pointing the "Community" entry in the top nav to the legacy Astropods Slack workspace. The team has moved to Discord, and the docs theme was also still on the early teal palette while the product has shifted to the indigo primary defined in `astro-theme`.

## Design

Two coordinated edits to `docs-public/fern/`:

**Community link.** The `navbar-links` entry in `docs.yml` now reads `Join Discord` and points at `https://discord.gg/68t2xzzRrE`. No icon — kept text-only so it sits cleanly next to the existing GitHub icon and "Website" CTA.

**Brand refresh.** Swapped the docs theme over to the indigo scale used by `packages/astro-theme`:

- `colors.accent-primary` in `docs.yml`: teal `#12766e` → indigo-700 `#3e3bc9` (light) / indigo-600 `#4948e6` (dark).
- `colors.background` in `docs.yml`: now indigo-50 `#edf2fe` (light) / indigo-950 `#1c1d48` (dark).
- Welcome hero Quickstart button in `styles.css`: teal `#0f766e`/`#115e59` → indigo-700/indigo-800.
- `hero.svg`: the outer container fills were swapped to indigo-700/indigo-800 to match the new accent. The illustration art inside the clip-path was deliberately left on its original palette (teals/blues/purples) to keep the artwork legible without re-authoring.
- `.hero-image` is now `transform: scale(1.15)` so the illustration reads a bit larger on the welcome page. Scaling via CSS (not the SVG `<g>` transform) avoids re-clipping interior geometry — an earlier in-SVG scale exposed overlap between the checkmark and the sphere below.

Hex values were derived by converting the OKLCH definitions in `packages/astro-theme/src/colors.ts` to sRGB via `culori`. Fern's `accent-primary` only accepts hex strings, so the conversion is one-way and frozen — if the theme indigo shifts later, these values will need updating in lockstep.

## Migration

No action required.
