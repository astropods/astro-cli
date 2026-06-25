## Summary

The eval grade sidebar now explains why a dataset has its current grade and what reviewers should do next. The goal is to make the grade feel less opaque: instead of only showing a letter and progress bar, the sidebar names the current bottleneck, such as low coverage, too few failure cases, or a healthy dataset.

## Design

The grade is based on an internal score that follows familiar letter-grade bands. The score is not just the good-case percentage. It blends three signals:

- scored volume, with full confidence around 100 good/bad labels
- good-case share across scored labels
- failure coverage, with roughly 10% bad cases as the target so evals know what to catch

The grade block now uses dataset-focused language throughout. The label changes from `Baseline grade` to `Dataset grade`, and the headline below the letter avoids technical baseline language:

- empty dataset: `Start grading`
- `A`: `Dataset looks healthy`
- `B`: `Improve your dataset`
- `C`, `D`, `F`: `Needs more coverage`

The client uses those same signals to choose a guidance card under the grade. The guidance is evaluated in this order so each dataset state lands in one clear case:

- `Start grading`: no good or bad labels exist yet. Body: `Label recent traces as good or bad. These labels determine how reliable this dataset is.`
- `Grade more cases`: fewer than 100 scored labels exist. Body: `More labels make the dataset score more reliable. Make sure to include some bad cases.`
- `Add failure cases`: at least 100 scored labels exist but fewer than roughly 10% are bad. Body: `Only N% of traces are labeled bad. Add failure cases so your dataset captures how the agent actually fails.`
- `Reduce noise`: more than roughly 25% of scored labels are bad. Body: `N% of traces are labeled bad. Add good examples or remove bad labels that don't reflect real failures.`
- `Dataset looks healthy`: the dataset has enough scored labels, a healthy failure mix, and an `A` grade. Body: `This dataset is a reliable signal. Keep grading as the agent's behavior changes.`
- `Improve your dataset`: the fallback for enough volume and a healthy failure mix below `A`. Body: `Continue grading traces to increase your dataset's reliability.`

The card uses the shared card primitive inside the existing dark eval sidebar structure, with a primary-tinted border, subtle background, and lightbulb icon. It keeps the grade letter and progress bar as the primary scan targets while adding a compact coaching message underneath.

## Migration

No user action is required. Existing eval summaries already include the counts, grade, next grade, and progress needed to render the guidance.
