# A chart-1/chart-2 semantic color pair, and the Usage page's illegible dot

## Summary

The Usage page's Compute/Models legend used the same `--primary` indigo dot
for both series, distinguished only by `opacity-40` on the Models one.
Alpha-blending a mid-brightness indigo down to 40% reads fine on light
backgrounds but nearly disappears against a dark one, so the Models dot was
effectively invisible in dark mode. Fixing the opacity was easy; landing on
a genuinely distinct, theme-safe second color for it took a few rounds.

## Design

Ruled out several options, each for a specific reason:
- **Postman orange** and **amber** — no orange scale exists separate from
  `amber`, and `amber` already means "warning / degraded / suspended"
  elsewhere in this app (`PodTile.tsx`). Reusing it for a neutral spend
  metric risks reading as a status, not a category.
- **A hardcoded lighter indigo**, and **`bg-foreground-accent`** (indigo-600
  light / indigo-400 dark) — both are still shades of the same hue as
  `--primary`. Two shades of one hue read as "the same color, slightly
  different" at legend-dot size, not as two distinct series.
- **Purple** — ~25° from indigo on the hue wheel, the same closeness
  problem under a different name.
- **A hardcoded teal**, first attempt — teal is a real hue difference
  (~84° from indigo) with genuine precedent in this app (`MetricCard.tsx`'s
  sparkline switches to `teal-500` in dark mode for the same reason:
  indigo doesn't read clearly there). But the first shade pick
  (`teal-600` light / `teal-400` dark) landed at nearly the same
  *lightness* as `--primary` in light mode (teal-600 ≈45.6% L vs.
  indigo-700 ≈45.7% L), so despite the hue gap it still read as too
  similar at a glance. Widened the gap to `teal-300` (light) / `teal-400`
  (dark), giving both a hue difference and a real lightness difference
  from `--primary` in each theme.

**Landed on a proper semantic token pair, not another one-off literal.**
Added `chart-1` (aliases `--primary`) and `chart-2` (`teal-300` light /
`teal-400` dark) to `packages/astro-theme`'s `lightTheme`/`darkTheme`
maps, wired into `astro-client`'s `@theme inline` block as
`--color-chart-1`/`--color-chart-2` so `bg-chart-1`/`bg-chart-2` are real
Tailwind utilities, and documented both in `apps/astro-client/CLAUDE.md`'s
Colors section and `Theme.stories.tsx`'s palette page. The next chart that
needs two distinguishable series reuses this instead of re-deriving it.

## Migration

No behavior change. Visual only: the Models dot is a full-opacity teal
instead of a dimmed indigo, clearly distinct from the Compute dot in both
themes. `packages/astro-theme` consumers rebuild `dist/` from `src/` as
usual; no manual step needed beyond the normal build.
