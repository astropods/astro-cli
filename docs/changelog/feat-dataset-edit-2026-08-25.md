# Dataset item edit and delete

## Summary

Adding a trace to the dataset stored a verified value for every evaluator, but nothing could change
or remove it afterwards. These two endpoints complete the dataset item lifecycle.

## Design

```http
PUT    /api/v1/deployments/:id/dataset/items/:trace_id/evaluator-outputs
DELETE /api/v1/deployments/:id/dataset/items/:trace_id
```

The edit replaces every evaluator output on the item and records the reviewer in
`verified_by_user_id`. It requires the item to be on the active evaluation set, since a retired set
has no resolvable contract to validate against. The body carries a value for every evaluator in the
set, each checked against that evaluator's declared output contract:

```json
{
  "values": [
    { "key": "exposed_pii", "value": false },
    { "key": "user_sentiment", "value": "negative" }
  ]
}
```

The delete removes the local rows and the Langfuse item, and restores the local rows if the upstream
delete fails. It never inspects the evaluation set, so a stale item can always be cleared. A trace
that is in neither place returns a 404 rather than reporting a delete that never happened. Evaluation
runs and their results are untouched by both endpoints.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. On `eval_dataset_items`,
`added_by_user_id` is replaced with a nullable `verified_by_user_id`. Nothing calls the add endpoint
yet, so there is no data to migrate.
