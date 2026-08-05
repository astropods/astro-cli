# Review queue keyboard workflow

## Summary

Reviewers can navigate the review queue and record verdicts without leaving the keyboard. Optional criteria remain available without blocking selection of another trace.

## Design

The queue is a single-select listbox that keeps DOM focus while Arrow Up and Arrow Down update the active trace. Arrow input from elsewhere on the page hands focus to the queue without scrolling it, and boundary keys are consumed without changing selection. Row clicks and header navigation use the same selection path.

`G`, `B`, and `S` select Good, Bad, and Not sure. Enter agrees with the judge prediction when one is available. Predicted and manual verdict controls share the same shortcut definitions and visual hints. Shortcuts ignore editable fields, modifier keys, repeated events, and keys already handled by another widget.

Verdicts with optional criteria open a non-modal dialog focused on the first criterion. Reviewers can save criteria or undo the verdict, while selecting another queue item dismisses the dialog and continues the review workflow.

## Migration

No migration is required.
