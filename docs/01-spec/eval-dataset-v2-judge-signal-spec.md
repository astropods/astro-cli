# Eval Dataset v2 — Judge Signal

Extends `docs/01-spec/eval-dataset-v2-spec.md` and `docs/01-spec/eval-dataset-v2-judgment-reasons-spec.md`.

## Summary

The eval dataset review queue currently shows a positive, negative, or no-signal indicator based on keyword sentiment in the user's next reply. That signal is too narrow: it does not account for explicit thumbs feedback, the trace input/output, or the dataset criteria reviewers use when tagging examples.

The product needs a stronger queue signal that predicts the dataset review verdict a human reviewer is likely to choose for a trace. This is a dataset-admission judge, not an eval judge for experiment runs over an existing dataset. A trace can be `bad` because it is a useful failure example, and `unknown` when it is not useful for the dataset even if the agent response is acceptable.

This spec uses an Astro-managed judge. `astro-server` owns prompt construction, quota checks, model execution, prediction storage, and queue ordering. Langfuse remains the source of traces, trace scores, and accepted dataset items, but Langfuse does not run the judge.

Astro-managed execution gives request-time quota control, avoids scoring traces users may never review, keeps queue ordering tied to the loaded candidate window, and lets the judge use Astro-local context without copying it into Langfuse metadata.

---

## Goals

- Predict a numeric verdict score from `-1` to `1`, then infer whether a queue trace is likely to be marked `good`, `bad`, or `unknown`.
- Use trace input/output, sentiment inferred from the next user message, thumbs feedback stored as Langfuse trace scores, dataset criteria, and prior resolved verdicts.
- Apply judge-specific quota through OpenMeter with a dedicated judge token usage meter.
- Avoid duplicate spend by reusing stored predictions when present.
- Use prediction quality against later resolved verdicts to improve prompts, calibration, and future model behavior.

## Non-goals

- Do not implement thumbs-up/thumbs-down capture. It is assumed to exist as a Langfuse trace score.
- Do not train or fine-tune a model for the first version.
- Do not replace Langfuse as the source of traces, trace scores, or accepted dataset items.
- Do not require every trace to have a prediction before it can appear in the queue.

---

## Current state

The review queue endpoint lives at:

`GET /api/v1/deployments/:id/dataset/review-queue`

It fetches traces from Langfuse with `fields=core,io`, filters out locally judged trace IDs, infers sentiment from the next user message in the same session, and sorts items with any sentiment before items with no signal.

Relevant local state already exists:

- `eval_datasets` maps deployments to Langfuse dataset names.
- `eval_dataset_judgments` stores reviewer verdicts: `good`, `bad`, `unknown`.
- `eval_dataset_judgment_reasons` stores reviewer-selected criteria values.
- Langfuse `TraceDetail` includes trace-level scores, where thumbs feedback will be read.

---

## Signal contract

Replace displayed `sentiment` with a dataset verdict signal inferred from a numeric prediction.

| Raw `verdict_score` | Inferred signal | Meaning |
| --- | --- | --- |
| `>= 0.25` | `good` | The trace is predicted to be accepted as a good dataset example. |
| `<= -0.25` | `bad` | The trace is predicted to be accepted as a bad/failure dataset example. |
| `> -0.25` and `< 0.25` | `unknown` | The trace is predicted to be skipped or lacks enough signal. |

## Judge input

For each trace, the judge should receive a compact structured view of:

- trace input
- trace output
- sentiment inferred from the next user message and short supporting text when available
- thumbs feedback score from Langfuse trace scores
- rubric dimensions: accuracy, completeness, instruction following, scope clarity, and tone
- small set of prior labeled examples from the same eval dataset, when available

Prior examples should be selected only from the same eval dataset, exclude the trace being judged, include both `good` and `bad` examples where possible, and be capped to keep token cost predictable.

System instruction:

Predict how a human reviewer will judge whether the trace belongs in this eval dataset. A `good` verdict means the agent output is good given the trace input. A `bad` verdict means the agent output is bad given the trace input. An `unknown` verdict means the trace is irrelevant, ambiguous, or not useful for the dataset.

Return an overall `verdict_score` on a scale from `-1` to `1`, where `1` means the reviewer is very likely to mark the trace `good`, `-1` means the reviewer is very likely to mark the trace `bad`, and scores near `0` mean `unknown`. Return `confidence` from `0` to `100`.

Return one criterion score for each rubric dimension. Each criterion score is on the same `-1` to `1` scale: positive values mean the trace is predicted to satisfy that criterion, negative values mean the trace is predicted to violate it, and values near `0` mean the criterion is unclear or not relevant. The overall `verdict_score` should be consistent with the criterion scores, but it is not a simple average; weigh criteria by how important they are to whether the trace is a useful `good`, `bad`, or `unknown` dataset example.

Return structured output with this shape:

```json
{
  "verdict_score": 0.0,
  "confidence": 0,
  "explanation": "Short explanation capped at 240 characters.",
  "criteria": [
    { "dimension_key": "accuracy", "dimension_value": 0.0 }
  ]
}
```

## Judge configuration

Define the V1 judge execution model and judge version as code-owned constants in `internal/evaljudge`:

- `EvalDatasetJudgeModel`
- `EvalDatasetJudgeVersion = "dataset-review-v1"`

`EvalDatasetJudgeVersion` identifies the prompt, parser, criteria handling, and model configuration used to produce a prediction. Users do not choose either value per dataset in V1.

---

## Prediction storage

Add `eval_dataset_judgment_predictions` in Astro DB as the source of product prediction state. Insert rows when predictions are generated, before any reviewer verdict exists, so later queue loads can reuse the model output without spending more tokens.

```sql
CREATE TABLE eval_dataset_judgment_predictions (
  eval_dataset_id uuid NOT NULL,
  trace_id text NOT NULL,
  verdict_score numeric NOT NULL,
  confidence integer NOT NULL,
  explanation text NOT NULL DEFAULT '',
  judge_version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT eval_dataset_judgment_predictions_pkey
    PRIMARY KEY (eval_dataset_id, trace_id),
  CONSTRAINT eval_dataset_judgment_predictions_dataset_fkey
    FOREIGN KEY (eval_dataset_id) REFERENCES eval_datasets(id) ON DELETE CASCADE,
  CONSTRAINT eval_dataset_judgment_predictions_score_check
    CHECK (verdict_score BETWEEN -1 AND 1),
  CONSTRAINT eval_dataset_judgment_predictions_confidence_check
    CHECK (confidence BETWEEN 0 AND 100),
  CONSTRAINT eval_dataset_judgment_predictions_explanation_check
    CHECK (char_length(explanation) <= 240)
);

CREATE TABLE eval_dataset_judgment_prediction_criteria (
  eval_dataset_id uuid NOT NULL,
  trace_id text NOT NULL,
  dimension_key text NOT NULL,
  dimension_value numeric NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT eval_dataset_judgment_prediction_criteria_pkey
    PRIMARY KEY (eval_dataset_id, trace_id, dimension_key),
  CONSTRAINT eval_dataset_judgment_prediction_criteria_prediction_fkey
    FOREIGN KEY (eval_dataset_id, trace_id)
    REFERENCES eval_dataset_judgment_predictions(eval_dataset_id, trace_id)
    ON DELETE CASCADE,
  CONSTRAINT eval_dataset_judgment_prediction_criteria_value_check
    CHECK (dimension_value BETWEEN -1 AND 1)
);
```

Column notes:

- `eval_dataset_id` and `trace_id` identify one prediction per trace within a dataset, matching `eval_dataset_judgments`.
- `verdict_score`, `confidence`, and `explanation` are the judge output stored for queue display and later comparison.
- `judge_version` is a marker for model and prompt performance analysis.
- `dimension_key` and `dimension_value` mirror `eval_dataset_judgment_reasons`. The server validates `dimension_key` against the existing criterion enum and `dimension_value` stays on the `[-1, 1]` scale.
- `created_at` and `updated_at` support audit and overwrite behavior.

---

## Model execution

Today, Astro uses AI Gateway for deployed agents and local dev agent runs. In those flows, `astro-server` only provisions virtual keys and injects them into agent environments; the agent process makes the model call. This feature expands AI Gateway usage to a platform-internal caller: `astro-server` will make the judge model call directly for dataset review predictions.

Required AI Gateway changes:

1. Add an account-scoped AI Gateway key table and store.
2. Add an account-scoped key provisioner for `eval_dataset_judge`.
3. Add an AI Gateway invocation client for OpenAI-compatible chat completions.

### 1. Account key table and store

Create a new account-scoped AI Gateway key table. Do not reuse `deployment_ai_gateway`, because that table is deployment-scoped and tied to agent runtime usage. This table should follow the same encrypted-key storage approach as `deployment_ai_gateway`, but the primary key is account and purpose scoped instead of deployment scoped.

```sql
CREATE TABLE account_ai_gateway_keys (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  key_id TEXT NOT NULL,
  encrypted_api_key TEXT NOT NULL,
  encrypted_data_key BYTEA,
  nonce BYTEA,
  issued_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, kind)
);
```

Column notes:

- `kind` is the key purpose, initially `eval_dataset_judge`.
- `key_id` is the LiteLLM key ID used for revoke/delete.
- `encrypted_api_key`, `encrypted_data_key`, and `nonce` follow the existing KMS envelope pattern used by deployment AI Gateway keys.

Add a store under `internal/aigateway` with:

- `Get(accountID, kind)`
- `Save(row)`
- `Delete(accountID, kind)`
- `ListByAccount(accountID)`

Account purge should revoke all listed LiteLLM `key_id` values before deleting the rows.

### 2. Eval-judge key provisioner

Add an account-scoped provisioner method, separate from deployment and dev key provisioning. Follow the `EnsureDeploymentKey` lifecycle:

- Reuse an existing `(account_id, eval_dataset_judge)` row when present.
- Otherwise call LiteLLM `/key/generate` using the existing `internal/aigateway.Client.GenerateKey`.
- Encrypt and store the returned plaintext key.
- Return plaintext key plus AI Gateway base URL for immediate invocation.
- Revoke upstream keys during account cleanup.

Key generation shape:

```go
resp, err := client.GenerateKey(ctx, aigateway.KeyRequest{
  UserID: accountID,
  TeamID: accountID,
  Metadata: map[string]any{
    "kind": "eval_dataset_judge",
    "source": "astro-server",
  },
})
```

`user_id` and `team_id` must be `accounts.id`, not the authenticated human user ID. This preserves the existing AI Gateway attribution invariant. Do not put dataset-specific values on the long-lived LiteLLM key metadata.

### 3. AI Gateway invocation client

Add a runtime invocation client in `internal/aigateway`. The existing client only covers LiteLLM admin APIs such as `/key/generate` and `/key/delete`. Runtime judge calls must use the eval-judge virtual key as the bearer token, not the AI Gateway master key.

Example request shape:

```json
{
  "model": "<configured judge model>",
  "stream": false,
  "messages": [
    { "role": "system", "content": "<judge instructions>" },
    { "role": "user", "content": "<structured judge input>" }
  ]
}
```

Return the raw assistant output, parsed prediction, and token usage from the response `usage` object. If `usage.total_tokens` is missing, treat the call as a metering failure.

---

## Queue behavior

The default queue should stay stable while predictions are generated.

Default queue load:

1. `GET /dataset/review-queue` returns queue items sorted by trace recency only.
2. Include stored predictions when available.
3. Return `prediction: null` when no stored prediction exists.
4. The client automatically calls `POST /dataset/review-queue/predictions` for rows with no prediction in the loaded window.
5. The client shows those rows as `pending` while the POST is in flight.
6. When the prediction response returns, update row signals in place without changing the default order.

Only successful predictions are stored in `eval_dataset_judgment_predictions` and `eval_dataset_judgment_prediction_criteria`. Runtime UI states such as `pending` and POST errors such as `quota_exhausted` and `prediction_failed` are derived from the client request state and prediction POST response; they are not stored in the prediction tables.

V1 should prefer sort controls over filters, because prediction coverage can be partial. Filters can make the visible list look complete when some rows have no prediction yet.

Sort options:

- Newest first (default)
- Likely good first
- Likely bad first
- Highest confidence

Prediction sort options should keep unpredicted rows visible, ordered by recency after predicted rows.

## Queue API

### `GET /dataset/review-queue`

Keep the existing queue endpoint read-only. It should continue fetching and filtering candidate traces, then add stored prediction data before returning. Default ordering stays trace-recency based.

Changes:

1. Sort queue items by trace recency only.
2. Replace the existing `sentiment` field with a nullable `prediction` field.
3. Populate `prediction` from stored predictions for returned trace IDs, or `null` when none exists.

Response shape change:

```json
{
  "items": [
    {
      "trace_id": "trace_123",
      "...existing_queue_fields": "...",
      "prediction": {
        "verdict_score": -0.72,
        "confidence": 84,
        "explanation": "The response misses the requested constraint.",
        "judge_version": "dataset-review-v1",
        "criteria": [
          { "dimension_key": "accuracy", "dimension_value": -0.8 }
        ]
      }
    },
    {
      "trace_id": "trace_456",
      "...existing_queue_fields": "...",
      "prediction": null
    }
  ]
}
```

### `POST /dataset/review-queue/predictions`

Add a quota-consuming prediction endpoint that accepts trace IDs from the loaded queue window, generates missing predictions, and stores the results.

Flow:

1. Resolve account, deployment, dataset, and requested trace IDs. Validate ownership, candidate review scope, judgment state, and max batch size.
2. Load stored predictions for the requested trace IDs. Return cached predictions with `error: null`; cached predictions do not require quota and do not emit new usage.
3. Process cache misses one trace at a time in V1. Before each model call, check OpenMeter access. If quota is exhausted, stop and return `error: "quota_exhausted"` for remaining misses.
4. For each allowed miss, ensure the account has an eval-judge AI Gateway key, call the AI Gateway with `stream=false`, parse and validate the output, store the prediction and predicted criteria, then emit `eval_judge_token_usage` immediately from the response usage.
5. Return one result per requested trace with a nullable `prediction` and nullable `error`.

Response shape:

```json
{
  "results": [
    {
      "trace_id": "trace_123",
      "prediction": {
        "verdict_score": -0.72,
        "confidence": 84,
        "explanation": "The response misses the requested constraint.",
        "judge_version": "dataset-review-v1",
        "criteria": [
          { "dimension_key": "accuracy", "dimension_value": -0.8 }
        ]
      },
      "error": null
    },
    {
      "trace_id": "trace_456",
      "prediction": null,
      "error": "quota_exhausted"
    },
    {
      "trace_id": "trace_789",
      "prediction": null,
      "error": "prediction_failed"
    }
  ]
}
```

---

## OpenMeter meter

Add a new OpenMeter meter and feature for judge token usage. Astro emits this event after each successful non-streaming judge model call using the usage fields returned by AI Gateway/LiteLLM.

Meter definition:

- slug: `eval_judge_tokens`
- event type: `eval_judge_token_usage`, emitted by `astro-server`
- aggregation: `SUM`
- value: `$.total_tokens`
- subject: Astro account ID
- group-by fields: `model`, `judge_version`

Event shape:

```json
{
  "type": "eval_judge_token_usage",
  "subject": "<account_id>",
  "data": {
    "total_tokens": 1240,
    "input_tokens": 1100,
    "output_tokens": 140,
    "model": "claude-haiku-...",
    "judge_version": "dataset-review-v1",
    "deployment_id": "dep_...",
    "eval_dataset_id": "...",
    "trace_id": "..."
  }
}
```

---

## Other solutions considered

### Langfuse-managed evaluator

This option would configure a Langfuse LLM-as-judge evaluator and live evaluation rule. Langfuse would run the judge for matching observations, write a numeric score, and Astro would read that score back for queue display and ordering.

Relevant API support exists:

- `GET /api/public/llm-connections`
- `PUT /api/public/llm-connections`
- `DELETE /api/public/llm-connections/{id}`
- `POST /api/public/unstable/evaluators`
- `GET /api/public/unstable/evaluators`
- `POST /api/public/unstable/evaluation-rules`
- `PATCH /api/public/unstable/evaluation-rules/{evaluationRuleId}`

It was not chosen for V1 because:

- Execution is async and tied to trace ingestion, not queue load.
- Broad evaluation rules can spend tokens on traces users never review.
- Quota and metering control are indirect. Astro can disable a rule, but queued or in-flight evaluations may still spend tokens, and product-specific usage would require gateway/Langfuse usage classification after the fact.
- Astro-local judge inputs, such as prior resolved examples and dataset review context, would need to be copied into Langfuse observation metadata before evaluation.
- Public evaluator and evaluation-rule APIs are unstable.
- Evaluation rules target observations/experiments, while the review queue is trace-level and would need a reliable root/final observation target.
- Confidence and a capped explanation are not first-class fields on one judge result without extra parsing or additional scores.

Langfuse-managed evaluation remains a possible future path if async pre-scoring and soft quota enforcement become acceptable.

### Astro-managed direct provider credentials

Astro could call OpenAI, Anthropic, or another provider directly using server-held credentials instead of the AI Gateway.

This is simpler than adding an eval-judge AI Gateway key store and invocation client, and it still gives Astro request-time quota checks. It was not chosen as the preferred path because it duplicates provider routing and credential management outside the AI Gateway. It is still a reasonable fallback if the AI Gateway changes are not worth the extra platform work for V1.

---

## Rollout

OpenMeter meter, feature, and plan grants are created outside the application PR sequence. Before enabling prediction generation, each environment that will run the feature must have the `eval_judge_tokens` meter and related feature/grants configured, and `docs/03-architecture/openmeter-integration.md` must be updated to document the new meter, event shape, feature, and entitlement behavior.

### PR 1 — Prediction storage

Add `eval_dataset_judgment_predictions` and `eval_dataset_judgment_prediction_criteria`, plus the store methods needed to read, upsert, and replace stored predictions and predicted criteria. This PR should not call a model or change queue behavior.

### PR 2 — AI Gateway platform invocation

Add `account_ai_gateway_keys`, account-scoped eval-judge key provisioning, account cleanup revoke behavior, and the AI Gateway chat-completions invocation client. This expands AI Gateway from deployed-agent runtime use to platform-internal model calls from `astro-server`.

### PR 3 — Judge service

Add `internal/evaljudge` with the judge constants, input assembly, prior-example selection, system instruction, structured-output parsing, validation, explanation cap, and criteria score handling. This PR should be testable with a fake model invocation client.

### PR 4 — Queue prediction API

Add `POST /dataset/review-queue/predictions`, reuse stored predictions, process cache misses one trace at a time, call the judge service, store successful predictions, and return per-trace `prediction` / `error` results. Keep prediction generation behind a disabled server capability until Astro-side OpenMeter integration is wired.

### PR 5 — Queue read API

Update `GET /dataset/review-queue` to sort by trace recency and include nullable `prediction` with stored predictions and predicted criteria for returned trace IDs. Keep the response backward-compatible until the client is updated in PR 7+; the existing client should continue to render without requiring the new prediction payload.

### PR 6 — Astro OpenMeter integration

Add Astro-side support for the externally configured `eval_judge_tokens` feature: entitlement copy, quota check helper, and `eval_judge_token_usage` event emission helper. Wire the quota check into the prediction endpoint, emit usage after successful model calls, enable the quota-consuming prediction flow, and update `docs/03-architecture/openmeter-integration.md`.

### PR 7+ — Client queue experience

After final UI design, update the review queue to automatically request predictions for visible rows without stored predictions, show pending state from client request state, derive the displayed signal from `prediction.verdict_score`, surface prediction confidence, capped explanation, and predicted criteria scores, and add sort controls for newest, likely good, likely bad, and highest confidence.
