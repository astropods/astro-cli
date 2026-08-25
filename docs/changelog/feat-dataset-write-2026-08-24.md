# Dataset item write

## Summary

Adding a trace to the dataset went through the judge, which recorded one good/bad verdict.
Evaluators replace that verdict with one typed value per evaluator, and nothing could store them.

This adds the endpoint that saves a trace to the dataset against the agent's active evaluation set,
with a verified value for every evaluator in it. The review queue calls it once the UI moves over.

## Design

```http
POST /api/v1/deployments/:id/dataset/items
```

```json
{
  "trace_id": "trace_123",
  "evaluation_run_id": "6f1599a0-d1d4-47eb-9c47-30e10ab81e80",
  "evaluator_outputs": [
    { "key": "exposed_pii", "value": false },
    { "key": "user_sentiment", "value": "negative" }
  ]
}
```

The request must carry a value for every evaluator in the active set, and each is checked against
that evaluator's declared output contract. A missing evaluator, an unknown key, or a value the
contract rejects fails the request with a 400.

`evaluation_run_id` is optional and names the run the reviewer had on screen. A run in any state is
accepted, as is no run at all; only a run belonging to a different trace, dataset, or evaluation set
is rejected with a 409.

Membership and outputs land in one local transaction, gated on the item's primary key so a repeat
add gets a 409, and the Langfuse dataset item is written after. A Langfuse failure deletes the local
rows and returns a 502.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. Adds `eval_dataset_items` and
`eval_dataset_item_evaluator_outputs`. Additive, no backfill.
