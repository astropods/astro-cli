## Summary

Two small refinements to the eval review queue detail view: the third verdict is now labelled "Skip" instead of "Neutral", and the detail header sits on a single line.

## Design

The neutral verdict option kept its underlying value (`unknown`) but was renamed "Skip" to better describe what reviewers are doing when they pass on a trace. Its keyboard shortcut moved from `N` to `S` to stay consistent with the new label.

The detail header was collapsed from two stacked rows into a single flex row with `justify-between`: the judgment buttons sit on the left, and the trace link, signal chip, and queue count group on the right. The "View trace" button lost its horizontal padding (and the compensating negative margin) so it aligns cleanly within the right-hand group.

## Migration

No action required.
