# Dataset page on evaluator values

## Summary

The review queue now saves a trace as a dataset item with the evaluator's values. The dataset page
still spoke judgments: it counted verdicts and edited criteria in a popover. This moves the dataset
page onto the item endpoints and removes the judgment criteria the UI no longer uses.

## Design

- The sidebar lists each evaluator with the number of items holding a value for it, expanding to the
  value distribution ranked by how many items hold each value.
- An item row shows its stored values, formatted the same way the review queue formats them.
- An item's values are edited in a modal built from the evaluation set, so an evaluator nobody answered
  still gets a control. Saving sends the values as they stand.
- An item admitted under a retired evaluation set cannot be edited, and the menu says why.

## Migration

None.
