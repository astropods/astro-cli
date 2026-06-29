## Summary

- Keep long eval review content manageable instead of letting prompts or outputs stretch the whole review queue.
- Apply the shared `dp-scroll` scrollbar treatment to eval tab scroll surfaces.
- Tighten the eval dataset guidance banner so it does not consume as much sidebar space.
- Make dataset grade progress more concrete by showing how many additional judged cases are needed for the next grade.
- Rename the sidebar grade label to "Dataset reliability".
- Render the eval dataset id in the card header as 11px Geist Mono.
- Use "response" terminology for judged output so reviewers are not asked to reason about "examples" in the expected-output field.

## Design

- Review queue cards:
  - Let the eval page own vertical scrolling for the selected review trace instead of adding a nested trace-pane scroller.
  - Keep long user input and agent response bodies manageable with optional per-section vertical resize.
  - Use a reusable resize-corner handle so resizable review content has the same affordance as other future resizable surfaces.
  - Keep the resize affordance keyboard-operable with arrow-key height adjustments and prevent touch drags from scrolling the page.
  - Use the shared `dp-scroll` utility on resizable content and eval scroll surfaces so scrollbars match the rest of the app.
  - Keep the review queue layout readable when a prompt or output is very long.
  - Avoid introducing a persistent resize preference system; the change is local to each review queue display.
- Eval page shell:
  - Apply `dp-scroll` to the main eval page scroller and horizontally scrollable tab strip.
  - Extend `dp-scroll` to style horizontal WebKit scrollbars as well as vertical ones.
- Dataset sidebar:
  - Reduce the guidance banner padding, icon size, title/body gap, and body line-height.
  - Let the guidance body use the full banner width so short messages wrap less aggressively.
  - Move the guidance banner below composition so the sidebar shows grade and label mix before coaching copy.
  - Remove the duplicate empty-state "Start grading" headline above the guidance banner.
  - Replace ambiguous percentage progress copy with lower-bound copy like "at least 21 mixed labels to D".
  - Compute the next-grade lower-bound case count in the server's canonical dataset scoring package.
  - Expose that count as `cases_to_next_grade` alongside `grade`, `next_grade`, and `next_grade_progress`.
  - Keep the client grade component presentational so display copy cannot drift from server scoring floors or constants.
  - Use "Dataset reliability" as the section label for the grade summary.

The server already has the canonical dataset score model. It looks at three things: good ratio, volume, and failure coverage. Good ratio is how many labeled cases are good vs bad. Volume means more labeled cases makes the score more trustworthy, up to the target volume. Failure coverage means a dataset with only good labels is considered incomplete, because evals need some bad/failure examples too. `cases_to_next_grade` asks: "What is the smallest number of additional labeled cases that could get this dataset to the next grade?"

For each possible added-case count, the server checks a few possible good/bad mixes. Since the score is best when the bad-label share is near the target bad share, it only needs to check all new cases good, all new cases bad, and the mixes around the target bad share. When one of those mixes reaches the next grade floor, that added-case count is returned. So the number is a lower bound, not a promise. If it says "at least 21 mixed labels to D", it means: "With enough volume and a useful mix of good and bad labels, 21 more labels is the earliest this dataset can reach D." If the next 21 labels are all good, or all noisy bad labels, the grade may not move as expected because the score also cares about failure coverage and quality mix.

- Dataset header:
  - Render the eval dataset id with 11px mono type for clearer distinction from the "Dataset" title.
  - Add a `text-mono-xs` typography token for that 11px mono treatment instead of using an arbitrary Tailwind class.
- Dataset copy:
  - Rename good/bad expected-output labels from "Good example" and "Bad example" to "Good response" and "Bad response".
  - Update related guidance copy so the eval surface consistently talks about responses.

## Migration

- No migration required.
- `cases_to_next_grade` is additive API response metadata; existing datasets derive it from current good/bad counts at read time and present it as a lower bound.
