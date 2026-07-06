# Dataset view: surface judgment criteria

## Summary

Reviewers already tag good/bad items with criteria (accuracy, completeness,
instruction-following, scope, tone), but those labels weren't shown anywhere in
the evals Dataset view. This surfaces them.

## Design

The data was already flowing to the client in item metadata (`judgment_criteria`),
so this is client-only.

- **Reason column** shows the first label inline plus a `+N` chip that reveals the
  full set on hover. The overflow reuses the Insights `OverflowPopover`, extended
  with an opt-in `trigger="hover"` (click stays the default).
- **Reasons section** in the grade sidebar lists each criterion with its count,
  most-frequent first, each with a tooltip. A Good/Bad toggle (defaulting to Bad)
  switches between the two verdict breakdowns; the section is hidden only when no
  criterion has any count, and shows an empty message when the selected verdict
  has none.

## Migration

None.
