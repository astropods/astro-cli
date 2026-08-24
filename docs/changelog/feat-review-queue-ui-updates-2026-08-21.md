# Review queue evaluation results

## Summary

The review queue rendered one verdict per trace. It now reads evaluations, so a reviewer
sees what each evaluator found before deciding whether a trace belongs in the dataset.

## Design

The queue list reports one thing per trace: whether an evaluation has been requested and
how it ended. Selecting a trace opens the breakdown for that trace alone, which keeps a
fifty-item page from carrying six evaluator definitions and hundreds of explanations it
will not show.

The breakdown is a table of evaluator, result, and confidence, with each evaluator's
reasoning beneath its name and its definition behind an info hint. It reports what the run
recorded rather than what is configured now, so an evaluator that has since left the set
still appears, and one that never ran does not. A run that returned nothing says so in the
same panel, because the reader is already looking there. The panel opens itself once a run
settles, since reading the results is the reason to select a trace, and stays wherever the
reader last put it.

Results stay current while the reader works. The selected trace refreshes while its
evaluation is in flight, and a run that finishes while the reader is elsewhere in the
queue shows its results when they come back to it. The list and the open trace never
disagree about whether a trace has been evaluated.

The interface says evaluate throughout, replacing judge.

## Migration

None for users.
