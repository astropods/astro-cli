# Fix the usage chart on the billing page

## Summary

The usage chart stacked every billable metric into one bar on one axis. The two
metrics measure different things: `Compute Units` sums `cu_hours` and `LLM Usage`
sums `cost_usd`. A bar's height was therefore compute hours plus dollars, a
quantity in no unit at all, and the y-axis had no unit it could name.

Three failures followed from that. Gateway spend runs one to two orders of
magnitude below compute hours, so the money series, which is what a billing page
is for, rendered as a cap a few pixels tall. Its color came from a local palette
of eight, five of whose variables do not exist in the theme; an undefined fill
falls back to SVG's default of black, so that series drew black and disappeared
against a dark background. And ticks were dropped wherever labels would collide,
leaving the last labelled day two behind the last bar, so the current day read as
missing.

## Design

**One panel per metric.** Each metric gets its own chart, its own axis, and its
own unit. Nothing is summed across units, and a small series is legible because
it is scaled against itself rather than against a larger one.

**A tick for every day.** Ticks carry the day of the month, which fits a full
billing period without collisions, so the current day is always labelled.

**One semantic color.** A single-series panel needs one color. It is
`--color-primary`, which flips with the theme, and the local palette is gone.
The tooltip moves to `--color-popover` for the same reason.

## Migration

None. Display only.
