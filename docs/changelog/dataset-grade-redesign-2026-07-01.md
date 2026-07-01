# Dataset grade sidebar: progress ring

## Summary

Redesign the eval dataset grade sidebar around a circular progress ring so the current grade and its distance to the next grade read at a glance.

## Design

- **Header** — "Dataset reliability" label and the horizontal good/bad composition bar are replaced by a compact "Baseline grade" header with a help tooltip explaining what the grade blends.
- **Ring** — a circular SVG ring centers the letter grade and captions it with `{n}% to {next_grade}` (or "Top grade" at A). The arc fills proportionally to `next_grade_progress` and is colored by grade tone.
- **Counts** — the good/bad counts move out of the sidebar; the dataset filter chips remain the count surface, so the sidebar stays focused on the grade.
- **Component** — `DatasetGrade`'s `label` variant is replaced by a `ring` variant taking `nextGrade` + `progress`; the `badge` variant (used in the page header) is unchanged.

No server or API changes: `next_grade_progress` was already computed and served.

## Migration

None.
