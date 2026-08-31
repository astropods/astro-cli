# Review queue on evaluator values

## Summary

The dataset endpoints moved to evaluator outputs, but the review queue still spoke judgments: a
reviewer picked good or bad and set criteria in a popover. This moves the review queue onto the item
endpoints and adds the shared pieces the dataset page reuses in the next PR.

## Design

- The review queue shows one panel per trace, scored or not: the evaluator's values, editable in place,
  and a single action that saves the trace as a dataset item.
- A scored trace opens with its results and confidence. A trace nobody evaluated is titled "Evaluate
  trace" and stays collapsed until the add action opens it.
- Saving sends the values as they stand, so partial and empty are both normal.
- A value the reviewer overrides is marked, keeping the evaluator's verdict distinct from the reviewer's.
- Every value is a dropdown, including booleans, so an unset value has a state. `SelectTrigger` grew an
  opt-in `onClear`, which Radix cannot express on its own.
- The evaluator controls, the override tracking, and the value formatting are shared, so the dataset
  page renders the same value the queue does.
- The dataset page still reads the judgment fields it renders today, so it ships unchanged here.

## Migration

None.
