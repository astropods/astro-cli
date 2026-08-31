# Remove the dead judgment and judge-prediction code

## Summary

The review queue and dataset page cut over to evaluator outputs
(`feat-review-queue-cutover-2026-08-27`, `feat-ui-cutover-2026-08-27`). After
that cutover, nothing calls the human good/bad/criteria judgment endpoints,
and nothing ever enqueued the automated judge-prediction worker's jobs in the
first place. This removes both, along with the storage and schema that only
they used.

## Design

Deleted outright: `handlers/dataset_judgments.go` and its four
`/dataset/judgments...` routes, the `judgmentstore` package (verdict storage
and judge-prediction storage), the `evaljudge` package (the judge-prediction
model invocation), `riverqueue/eval_judge_prediction.go`
(`EvalJudgePredictionWorker`, queue `eval-judge`), and the
`eval_datasets.good_count`/`bad_count` counters those judgments bumped.

Dropped from `sql/astro-server/schema.sql`: `eval_dataset_judgments`,
`eval_dataset_judgment_reasons`, `eval_dataset_judgment_predictions`,
`eval_dataset_judgment_prediction_criteria`, `eval_dataset_prediction_requests`,
and the `good_count`/`bad_count` columns.

A few pieces `eval_judge_prediction.go` defined are still load-bearing for the
evaluator flow that replaced it: the billing-suspension gate and some trace
text/feedback helpers. Those moved into `riverqueue/eval_dataset_evaluation.go`,
the live evaluator worker, rather than being deleted.

## Migration

Apply the updated `sql/astro-server/schema.sql` before or alongside this
deploy. It drops five tables and two columns; there is no data migration
because nothing has written to them since the client cutover.
