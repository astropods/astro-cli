# Judgment criteria selection in the review queue

## Summary

Reviewers could mark a queue trace good/bad/neutral but had no way to record *why*. The backend
already stored judgment criteria and exposed a `PUT .../criteria` endpoint; this adds the frontend
that lets a reviewer optionally select criteria after marking a trace, closing the loop so criteria
reach Langfuse dataset-item metadata and the summary's criteria counts.

## Design

Marking a trace still posts the verdict alone. The confirmation popup — previously a small
Undo-only toast — now expands into a criteria selector for good/bad verdicts: a "Why is it
good/bad?" prompt, an Optional badge, a wrapping set of toggleable chips, and a Done button.
Clicking Done sends the selected criteria to the existing `PUT .../criteria` endpoint; Done with no
selection just dismisses. Criteria are optional by design.

The five criterion dimensions are fixed by a server enum (`accuracy`, `completeness`,
`instruction_following`, `scope_clarity`, `tone`); the frontend owns their display labels and order
and submits `value: 1` for good, `-1` for bad. Chip labels switch to the negative wording for a bad
verdict.

The good/bad popup no longer auto-dismisses on a timer — it closes only on Done or Undo, so a
reviewer has time to choose criteria. The judged trace stays on screen until Done: the queue only
drops it and advances to the next trace once the reviewer finishes, keeping context while they pick
criteria. The verdict flight animation still plays at the moment of judging. Neutral verdicts keep
the original compact quick-undo toast with its auto-dismiss timeout.

The chip is a new shared `SelectableChip` primitive (a tone-aware toggle pill built on the existing
Button). The dataset filter chips were refactored onto the same primitive so both surfaces share one
implementation rather than duplicating the pattern.

## Migration

None. New criteria selection is optional and additive; existing judgments and the filter chips are
unaffected.
