# Eval Dataset v2 — Judge Signal

Extends `docs/01-spec/eval-dataset-v2-spec.md` and `docs/01-spec/eval-dataset-v2-judgment-reasons-spec.md`.

## Summary

The eval dataset review queue currently shows a positive, negative, or no-signal indicator based on keyword sentiment in the user's next reply. That signal is too narrow: it does not account for explicit thumbs feedback, the trace input/output, or the dataset criteria reviewers use when tagging examples.

The product needs a stronger queue signal that predicts the dataset review verdict a human reviewer is likely to choose for a trace. This is a dataset-admission judge, not an eval judge for experiment runs over an existing dataset. A trace can be `bad` because it is a useful failure example, and `unknown` when it is not useful for the dataset even if the agent response is acceptable.

This spec uses an Astro-managed judge. `astro-server` owns prompt construction, model execution, prediction storage, and queue ordering. Langfuse remains the source of traces, trace scores, and accepted dataset items, but Langfuse does not run the judge.

Astro-managed execution runs the judge at queue load rather than trace ingestion, avoids scoring traces users may never review, keeps queue ordering tied to the loaded candidate window, and lets the judge use Astro-local context without copying it into Langfuse metadata.

---

## Goals

- Predict a numeric verdict score from `-1` to `1`, then infer whether a queue trace is likely to be marked `good`, `bad`, or `unknown`.
- Use trace input/output, sentiment inferred from the next user message, thumbs feedback stored as Langfuse trace scores, dataset criteria, and prior resolved verdicts.
- Give judge traffic a stable account-scoped gateway identity so future platform AI-token metering can distinguish it from ordinary AI Gateway traffic.
- Avoid duplicate spend by reusing stored predictions when present.
- Use prediction quality against later resolved verdicts to improve prompts, calibration, and future model behavior.

## Non-goals

- Do not implement thumbs-up/thumbs-down capture. It is assumed to exist as a Langfuse trace score.
- Do not train or fine-tune a model for the first version.
- Do not replace Langfuse as the source of traces, trace scores, or accepted dataset items.
- Do not require every trace to have a prediction before it can appear in the queue.
- Do not implement or fully specify the Bifrost metering plugin, Metronome AI-token metrics, products, rate cards, invoice presentation, or pricing. That platform work requires its own spec.

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

Astro calls the judge model through the Bifrost AI Gateway. Today `astro-server` only provisions virtual keys for agent and development environments; this feature also makes model calls from `astro-server`. The two gateway interactions use different URLs and authentication:

- Governance (control plane): mint/revoke the judge virtual key via Bifrost's governance API at `AI_GATEWAY_ADMIN_URL` with the Basic admin header (`AI_GATEWAY_ADMIN_AUTH`). This reuses the existing `internal/aigateway` client (`GenerateKey`, `DeleteKey`, `CreateCustomer`).
- Model invocation: POST `${AI_GATEWAY_URL}/v1/chat/completions` with the judge virtual key as the bearer token. Use a separate invocation client; see below.

### Budget and metering

**The judge service does no metering and no spend gating of its own.** For the current launch, the judge key shares the account's temporary Bifrost customer budget with agent-runtime and development keys. `astro-server` never calls the billing seam and does not depend on the response `usage` (a missing `usage` never fails a prediction; record counts locally for observability if present).

Astro's billing integration is moving from OpenMeter to Metronome behind the provider-neutral `billing.BillingProvider` seam. AI Gateway tokens are not tracked by either integration today, so token metering will be new platform-wide work rather than judge-specific work. See `docs/01-spec/metronome-billing-spec.md`.

Only a confirmed customer-budget exhaustion response maps to `quota_exhausted` and stops a prediction batch. Judge tokens are not tracked in Metronome today, matching other AI Gateway traffic. Eventually, AI-token usage will be tracked in Metronome through separately specified platform-wide gateway metering; the plugin, event, metric, product, pricing, and invoice design are out of scope here.

Notes:

- The judge virtual key is deterministically named `eval-judge/<account-id>`, giving future platform metering a stable authenticated key ID/name with which to recognize judge traffic. Future work must not depend on parsing the free-form Bifrost description.
- No separate Bifrost customer or judge-specific budget is needed; the judge uses the same account-level gateway controls as other AI traffic.

### Future AI-token metering considerations — out of scope

The judge launch does not require the Bifrost metering plugin, Metronome AI-token metrics, usage-component events, or any Metronome product/rate-card changes. Those dependencies must not block enabling the judge behind its capability flag while the temporary Bifrost customer budget is active.

The separate platform AI-token metering spec must account for judge traffic and decide:

- how the plugin classifies the authenticated `eval-judge/<account-id>` key without parsing its description
- which event properties preserve account, model, judge-versus-ordinary usage, and optional `judge_version` attribution
- which Metronome group keys must be present when the AI-token metrics are created so usage can remain combined or be reported or priced separately later
- how input/output usage is represented and how the plugin prevents duplicate events or double billing
- how initial pricing and invoice presentation combine judge and ordinary AI usage while preserving the option to differentiate them later

The judge service does not emit billing events and does not require response-level `usage` for prediction success. The future platform-metering spec owns the exact plugin, event, metric, group-key, product, rate-card, and validation contracts.

### Gateway implementation

The gateway integration requires an account-scoped judge key store, a provisioner that reuses the account's Bifrost customer, and a model invocation client for OpenAI-compatible chat completions.

#### Judge key table and store

Add `account_llm_judge_keys`, one row per account, following the encrypted-key shape of the existing `deployment_ai_gateway` and `account_ai_gateway_dev_keys` tables. Do not reuse those tables (deployment- and user-scoped respectively) and do not reintroduce a generic account-scoped key table — the prior `account_ai_gateway` was already retired in favor of `deployment_ai_gateway`.

```sql
CREATE TABLE account_llm_judge_keys (
  account_id uuid NOT NULL,
  key_id text NOT NULL,
  encrypted_api_key text NOT NULL,
  encrypted_data_key bytea,
  nonce bytea,
  issued_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT account_llm_judge_keys_pkey PRIMARY KEY (account_id),
  CONSTRAINT account_llm_judge_keys_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
```

Column notes:

- `key_id` is the Bifrost virtual-key UUID, used for revoke/delete.
- `encrypted_api_key`, `encrypted_data_key`, and `nonce` follow the KMS envelope pattern used by the existing gateway key tables.

Add a store under `internal/aigateway` with `Get(accountID)`, `Save(row)`, `Delete(accountID)`, and `ListKeyIDsByAccount(accountID)`. Account purge must revoke the upstream Bifrost `key_id` before deleting the row, alongside the existing `RevokeAccount` / `RevokeAccountDevKeys` sweeps — Bifrost has no FK back to Astro, so the DB cascade alone leaves an orphaned upstream key.

#### Judge key provisioner

Add a judge key method on `aigateway.Provisioner`, modeled on `EnsureDevKey`:

- Resolve the account's Bifrost customer via the existing `ensureCustomer` (creates once, persists `accounts.bifrost_customer_id`). Attach the judge key to this customer — do not create a separate customer. The key therefore shares the account's current gateway budget (see [Budget and metering](#budget-and-metering)).
- Reuse the stored judge key when present; otherwise call `GenerateKey`, KMS-encrypt, and persist. Extend `KeyRequest`/`GenerateKey` so this key is named `eval-judge/<account-id>` and its Bifrost provider config restricts `allowed_models` to `EvalDatasetJudgeModel` rather than `*`.
- Return plaintext key plus the public gateway base URL for immediate invocation.
- Treat as long-lived like deployment keys: no TTL, no rotation. Revoke upstream on account purge.

The deterministic name distinguishes the key in the Bifrost admin view and preserves a stable identity for future platform metering. Do not put dataset- or trace-specific values on the long-lived key name or description.

#### Model invocation client

Add a model invocation client in `internal/aigateway`, separate from the governance client, which only covers Bifrost admin APIs.

- POST `${AI_GATEWAY_URL}/v1/chat/completions`. The gateway base URL is the host only; append `/v1` (the bare host returns 404).
- Bearer token is the judge VK plaintext, not the Basic admin auth.
- OpenAI-compatible body with `stream=false`. The model is a Bedrock model served by the gateway (`EvalDatasetJudgeModel`).

Request schema-enforced output through Bifrost with `response_format.type=json_schema`. Use the complete prediction schema below: every property is required, `additionalProperties=false` at each object level, and `dimension_key` is restricted to the existing criterion enum. Numeric ranges, the 240-character explanation cap, and the requirement for exactly one unique result for each of the five dimensions remain server validations.

```json
{
  "model": "<configured judge model>",
  "stream": false,
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "eval_dataset_judgment_prediction",
      "strict": true,
      "schema": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "verdict_score": { "type": "number" },
          "confidence": { "type": "integer" },
          "explanation": { "type": "string" },
          "criteria": {
            "type": "array",
            "minItems": 5,
            "maxItems": 5,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "dimension_key": {
                  "type": "string",
                  "enum": [
                    "accuracy",
                    "completeness",
                    "instruction_following",
                    "scope_clarity",
                    "tone"
                  ]
                },
                "dimension_value": { "type": "number" }
              },
              "required": ["dimension_key", "dimension_value"]
            }
          }
        },
        "required": [
          "verdict_score",
          "confidence",
          "explanation",
          "criteria"
        ]
      }
    }
  },
  "messages": [
    { "role": "system", "content": "<judge instructions>" },
    { "role": "user", "content": "<structured judge input>" }
  ]
}
```

Before enabling the capability in an environment, smoke-test this exact model and schema through Bifrost's OpenAI-to-Bedrock translation. Do not silently fall back to free-text parsing: an absent or malformed structured result is `prediction_failed`.

Success is a valid parsed prediction; `prediction_failed` covers HTTP errors, timeouts, and unparseable or invalid output. The judge does not require `usage` — it is billing telemetry the gateway owns, so a missing `usage` never fails a prediction (record counts locally for observability if present).

---

## Queue behavior

The default queue should stay stable while predictions are generated.

Default queue load:

1. `GET /dataset/review-queue` returns queue items sorted by trace recency only.
2. Include stored predictions when available.
3. Return `prediction: null` when no stored prediction exists.
4. The client automatically calls `POST /dataset/review-queue/predictions` for rows with no prediction in the loaded window, chunked into at most 10 trace IDs per request.
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
2. Add a nullable `prediction` field. Retain the existing `sentiment` field until the client migration is complete, then remove it.
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

Add a prediction endpoint that accepts at most 10 trace IDs from the loaded queue window, generates missing predictions, and stores the results.

Flow:

1. Resolve account, deployment, dataset, and requested trace IDs. Validate ownership, candidate review scope, judgment state, uniqueness, and the 10-ID batch limit.
2. Load stored predictions for the requested trace IDs. Return cached predictions with `error: null`; cached predictions make no gateway call and produce no duplicate usage.
3. Process cache misses serially in request order. If the request context is canceled, stop before the next model call and do not persist or return a synthetic result for work that did not complete.
4. For each miss, ensure the account has its judge gateway key, POST to `${AI_GATEWAY_URL}/v1/chat/completions` with `stream=false`, validate the structured output, and atomically store the prediction and criteria.
5. Treat gateway or provider throttling as transient. If it cannot be recovered, return `prediction_failed` for that trace and continue the batch. While the shared gateway budget is configured, return `quota_exhausted` for the current and remaining misses only for a distinct structured Bifrost customer-budget exhaustion error; do not infer budget exhaustion from a generic HTTP 429 or message text.
6. Return one result per completed requested trace with a nullable `prediction` and nullable `error`. The capability flag controls whether generation is available during rollout; it is not a billing gate.

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

Langfuse-managed evaluation remains a possible future path if async pre-scoring and less direct spend attribution become acceptable.

### Astro-managed direct provider credentials

Astro could call OpenAI, Anthropic, or another provider directly using server-held credentials instead of the AI Gateway.

This is simpler than adding an eval-judge AI Gateway key store and invocation client. It was not chosen because it duplicates provider routing and credential management outside the AI Gateway and would not share the account's gateway budget. It remains a possible fallback if the gateway integration is not viable for V1.

---

## Rollout

Prediction generation has no external metering prerequisite: implement the prediction tables, Bifrost judge key and invocation path, judge service, APIs, and client experience in PRs 1–6. The existing temporary Bifrost customer budget covers judge spend; do not create the metering plugin or any Metronome AI-token objects as part of these PRs.

Platform-wide AI-token metering is deferred to its own spec and rollout. That work must consume the stable judge-key identity and address the considerations above, but it does not block the judge launch.

### PR 1 — Prediction storage

Add `eval_dataset_judgment_predictions` and `eval_dataset_judgment_prediction_criteria`, plus the store methods needed to read, upsert, and replace stored predictions and predicted criteria. This PR should not call a model or change queue behavior.

### PR 2 — Bifrost platform invocation

Add `account_llm_judge_keys`, the judge key provisioner (reusing `ensureCustomer`, deterministic `eval-judge/<account-id>` naming, judge-model allow-list), account-purge revoke behavior, and the `/v1/chat/completions` model invocation client. This expands Bifrost gateway use from deployed-agent and development runtime to platform-internal model calls from `astro-server`.

### PR 3 — Judge service

Add `internal/evaljudge` with the judge constants, input assembly, prior-example selection, system instruction, structured-output parsing, validation, explanation cap, and criteria score handling. This PR should be testable with a fake model invocation client.

### PR 4 — Queue prediction API

Add `POST /dataset/review-queue/predictions`, enforce the 10-ID limit, reuse stored predictions, process cache misses serially with cancellation and typed gateway errors, store successful predictions, and return per-trace `prediction` / `error` results. Judge calls go through Bifrost with the account's current shared temporary budget; keep generation behind a disabled server capability until the judge service and key provisioning land.

### PR 5 — Queue read API

Update `GET /dataset/review-queue` to sort by trace recency and include nullable `prediction` with stored predictions and predicted criteria for returned trace IDs. Retain `sentiment` temporarily so the existing client continues to render without requiring the new prediction payload.

### PR 6+ — Client queue experience

After final UI design, update the review queue to request predictions for visible rows without stored predictions in chunks of at most 10, show pending state from client request state, derive the displayed signal from `prediction.verdict_score`, surface prediction confidence, capped explanation, and predicted criteria scores, and add sort controls for newest, likely good, likely bad, and highest confidence. Remove `sentiment` from the queue response after the client migration is deployed.
