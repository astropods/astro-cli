# Agent evaluation set endpoint

## Summary

A reviewer is going to evaluate a trace by hand before adding it to the dataset. The frontend
cannot build that form without knowing what the current evaluators are: which ones exist, what to
label them, and what kind of value each one takes.

This adds a read for it. Nothing calls the endpoint yet; the modal that consumes it lands with the
dataset item write.

## Design

```http
GET /api/v1/agents/:account/:name/evaluation-set
```

```json
{
  "evaluation_ref": "preset/default-evaluation",
  "evaluators": [
    { "key": "exposed_pii", "label": "Exposed PII", "type": "llm",
      "output": { "type": "boolean" } },
    { "key": "user_sentiment", "label": "User sentiment", "type": "llm",
      "output": { "type": "enum", "options": ["positive", "neutral", "negative", "unclear"] } }
  ]
}
```

Evaluators come back in definition order. Each `output` is the typed contract a caller renders a
control from and the server validates a submitted value against.

The route is agent-scoped, and every agent resolves to the code-owned default set until published
sets exist. Account membership is the access gate.

## Migration

None. New endpoint, no existing behavior changes.
