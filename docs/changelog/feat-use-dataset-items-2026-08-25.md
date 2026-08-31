# Dataset reads on the item and output tables

## Summary

The writes already record `eval_dataset_items` and `eval_dataset_item_evaluator_outputs`, but the reads
still answered from the judgment tables: a trace added to the dataset stayed in the review queue, the
item list carried no evaluator values, and the summary counted verdicts nothing writes. This moves the
reads onto the item and output tables.

## Design

### `GET /dataset`

Verdict counts become one entry per evaluator, holding the distribution of the values reviewers
verified. It is a group-by over stored values, so an evaluator no item holds a value for is absent.

```diff
- "criteria_counts": [{ "dimension_key": "exposed_pii", "good_count": 38, "bad_count": 2 }]
+ "evaluators": [{ "key": "exposed_pii", "label": "Exposed PII",
+   "distribution": [{ "value": false, "count": 38 }, { "value": true, "count": 2 }] }]
```

### `GET /dataset/items`

Paging still comes from Langfuse, so identity, content, and pagination are unchanged. One local read
per page replaces the judgment `metadata` field.

```diff
- "metadata": { "judged_by_user_id": "user_1", "judgment_criteria": [...] }
+ "evaluation_ref": "preset/default-evaluation",
+ "verified_by_user_id": "user_1",
+ "evaluator_outputs": [{ "key": "exposed_pii", "label": "Exposed PII", "value": false }]
```

### `GET /dataset/review-queue`

A trace leaves the queue once it is a dataset item: the queue asks the item tables which trace IDs on
the page are already added, in place of the judgment lookup. `POST /dataset/evaluations` uses the same
rule, so the run action no longer picks a trace already in the dataset. A trace only the judgment path
recorded returns to the queue until the frontend calls the item endpoints.

### `GET /dataset/review-queue/:trace_id/evaluation`

The run gains the `id` a later add request sends back, and the results arrive in the set's order.

```diff
- "run": { "status": "completed", "error": null }
+ "run": { "id": "run_01J...", "status": "completed", "error": null }
```

### `POST /dataset/items` and `PUT /dataset/items/:trace_id/evaluator-outputs`

Both accept a value for as many evaluators in the set as the reviewer is sure of, including none. An
evaluator they left out has no row rather than a rejected request. Values are still checked against
their evaluator's output contract, and an unknown key is still a 400.

### `GET /agents/:account/:name/evaluation-set`

Each evaluator carries its `description`, so a trace no evaluator scored can still explain what each
value means.

## Migration

None. No schema change, and the judgment tables and endpoints are untouched.

The frontend cutover ships as its own stacked PRs, reviewed separately and released together. The
summary replaces `criteria_counts` with `evaluators`, and the deployed dataset grade panel reads the
field that goes away, so this merges ahead of the client work and deploys with it.
