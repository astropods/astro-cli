## Summary

Dark mode color polish pass. The existing teal palette was too vivid and recognisably "teal" in dark contexts. This shifts the palette toward a muted slate-teal and fixes several components that were either invisible or using off-palette colours in dark mode.

## Design

**Teal palette shift** — chroma reduced 50%, hue shifted +20° toward slate (194° → ~214°), using the chroma²-weighted mean as the center point to preserve the original regression-smoothed hue progression. Light mode is minimally affected; the primary button shade (teal-800) retains its character at reduced saturation.

**MetricCard** — was using `bg-white` (invisible on dark surfaces) and a hardcoded `--color-teal-600` for the sparkline stroke. Now uses `dark:bg-surface` and `--primary` so both respond to the theme.

**OrgSwitcher** — dark hover was `stone-700`, a warm brown that clashed with the new cool background. Replaced with `teal-800`.

**FilterInput** — placeholder text and search icon were `muted-foreground` (`teal-300`), which reads as a strong colour cast on a search field. Now `teal-25` (near-white) in dark mode.

## Migration

No action required.
