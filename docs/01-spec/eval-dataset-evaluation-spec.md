# Eval Dataset — Evaluation

Supersedes the model-prediction and human-judgment contracts in `docs/01-spec/eval-dataset-v2-judge-signal-spec.md` and `docs/01-spec/eval-dataset-v2-judgment-reasons-spec.md`, and — though not previously declared here — the grade and Langfuse-metadata model in [`eval-dataset-v2-spec.md`](eval-dataset-v2-spec.md), whose `good_count`/`bad_count` and grade formula this spec's own "Removed tables and fields" section deletes.

> **Status: the evaluator flow shipped, including the write-path
> replacement.** `internal/evalpreset`, the review-queue integration, and
> the client's writes through this flow's own `POST .../dataset/items` are
> all live. The judgment flow this spec proposes retiring is gone: its
> tables, handlers, and the judge-prediction worker are removed (see
> "Removed tables and fields" below). The part of this spec that remains
> unshipped is per-agent `EVALUATION.yaml`; every agent still uses the same
> hardcoded default evaluation set. See
> [`../03-architecture/traces-to-eval-dataset.md`](../03-architecture/traces-to-eval-dataset.md)
> for the system as it actually runs today.

## Summary

Agents have different purposes, risks, and definitions of quality, so one fixed judge cannot provide the information every builder needs when curating an evaluation dataset. Builders need to evaluate traces along dimensions specific to their agent while retaining useful platform-provided checks.

This specification introduces an evaluation set composed of independent evaluators. Each evaluator answers one defined question and returns a typed value, allowing measurements such as accuracy, user sentiment, policy compliance, classification, or a numeric score to coexist without being collapsed into one verdict.

Builders may select Astro-maintained presets or define custom evaluators through agent-scoped `EVALUATION.yaml`.

---

## Goals

- Let builders select preset evaluators or define custom evaluators.
- Support independent, typed evaluator results for each trace.
- Run every selected evaluator from one trace-level job.
- Display evaluator results without deriving a judgment or dataset decision.
- Let reviewers verify or edit evaluator outputs while adding a trace to the dataset.
- Let users dismiss traces from the review queue without adding them to the dataset.
- Record the evaluation reference and execution metadata for each result.
- Leave the evaluator definition extensible to classifier and code evaluators while supporting only LLM evaluators in version 1.

## Non-goals

- Automatically adding or removing traces based on evaluator results in version 1.
- Human verdicts, judgment reasons, or separate human-only evaluation dimensions.
- Combining multiple evaluators into one model request.
- Builder-selected models, temperatures, token limits, retry policies, or timeouts.
- Classifier or code evaluator execution in version 1.
- Evaluator authoring in the web UI.
- Independently versioning prompt files referenced by a custom evaluator.

---

## Terminology

- **Evaluation set:** An ordered collection of evaluators selected for an agent.
- **Evaluator:** An independent definition that analyzes a trace and returns one typed value.
- **Preset evaluator:** An evaluator maintained by Astro in code and selected by reference.
- **Custom evaluator:** An evaluator defined inline by a builder in `EVALUATION.yaml`.
- **Evaluation reference:** The stable identifier for the default or a builder-published evaluation set.
- **Evaluation run:** One execution of an evaluation set for one trace.
- **Evaluator result:** The value, metadata, or failure produced by one evaluator within a run.
- **Dataset evaluator output:** The final evaluator value stored with a dataset item, including its automated or human provenance.
- **Dataset item:** A trace a reviewer added to the evaluation dataset.
- **Dismissed trace:** A trace the user explicitly removed from the review queue without adding it to the dataset.

---

## Evaluation document

### Discovery

The filename is `EVALUATION.yaml` or `EVALUATION.yml`, located beside `astropods.yml`.

The CLI includes the file as optional agent-scoped configuration in the existing registration request made by `ast push`. No evaluation path is added to `astropods.yml`.

- An agent without a published evaluation set uses Astro's current default set.
- A supplied file defines the complete evaluation set and is not merged with Astro's defaults.
- A current CLI sends `evaluation: null` when the file is absent, restoring the default set. Older clients that omit the field leave the active set unchanged.
- An invalid file, unknown preset reference, or invalid referenced prompt file rejects the evaluation publication without changing the agent's active set.
- Filename matching is exact and case-sensitive.

### Format

`EVALUATION.yaml` declares its schema version and an ordered list of evaluators. This example shows the complete custom-evaluator structure, including optional context configuration:

```yaml
schema: evaluation/v1

evaluators:
  - key: has_secrets
    label: Contains secrets
    description: Flags credentials, API keys, tokens, or other secrets in the output.
    type: llm
    prompt: Determine whether the agent output exposes credentials, API keys, tokens, or other secrets.
    output:
      type: boolean

  - key: user_sentiment
    label: User sentiment
    type: llm
    config:
      context:
        previous_turns: true
        next_user_message: true
        user_feedback: true
    prompt: Determine the user's sentiment toward the agent response.
    output:
      type: enum
      options:
        - positive
        - neutral
        - negative
        - unclear

  - key: response_quality
    label: Response quality
    type: llm
    prompt_file: evaluation/response-quality.md
    output:
      type: number
      minimum: 0
      maximum: 1
```

The document fields are:

- `schema`: The evaluation document contract version. Version 1 is `evaluation/v1`.
- `evaluators`: The ordered evaluators to execute for each trace.

Each custom evaluator contains:

- `key`: A stable machine identifier used in persistence and APIs.
- `label`: A human-readable name.
- `description`: Optional human-readable explanation shown alongside the evaluator's results.
- `type`: The execution mechanism. Version 1 supports `llm`.
- `config`: Optional configuration defined by the evaluator type. For LLM evaluators, version 1 only supports `context`.
- `prompt` or `prompt_file`: The instructions used to perform the evaluation.
- `output`: The schema of the evaluator's returned value.

Evaluator order controls display and execution order but does not create dependencies between evaluators.

### Custom LLM evaluators

For `type: llm`, inline and file-backed prompts are mutually exclusive:

```yaml
prompt: Determine whether the response contains unsupported claims.
```

```yaml
prompt_file: evaluation/unsupported-claims.md
```

A prompt file contains the complete evaluator rubric. During normalization, its UTF-8 Markdown contents replace `prompt_file` as the evaluator's `prompt`; the path is not retained in the normalized definition.

### LLM configuration

Astro owns the model, temperature, token limit, structured-output behavior, timeout, retry policy, and concurrency in version 1. The only builder-configurable LLM behavior is additional trace context.

Every evaluator always receives the target trace input and output. Additional context defaults to excluded:

```yaml
config:
  context:
    previous_turns: false
    next_user_message: false
    user_feedback: false
```

Builders only need to specify values that differ from the defaults:

```yaml
config:
  context:
    previous_turns: true
    next_user_message: true
    user_feedback: true
```

- `previous_turns` includes completed earlier traces from the same session, subject to platform-owned count and size limits.
- `next_user_message` includes the next user-authored message when available.
- `user_feedback` includes available structured user feedback, such as thumbs-up or thumbs-down.

Context is supplied as data. It cannot alter the system-owned execution instructions or output schema.

### Output schemas

Each evaluator returns one value. Version 1 supports `boolean`, `enum`, `number`, and `string`.

#### Boolean

```yaml
output:
  type: boolean
```

Boolean outputs have no type-specific configuration.

#### Enum

```yaml
output:
  type: enum
  options:
    - positive
    - neutral
    - negative
    - unclear
```

Enum outputs require 2–20 unique options. Each option must contain 1–50 characters.

#### Number

```yaml
output:
  type: number
  minimum: 0
  maximum: 1
```

`minimum` and `maximum` are optional finite numbers. When both are present, `minimum` must be less than `maximum`.

#### String

```yaml
output:
  type: string
  max_length: 1000
```

`max_length` may be 1–4,000 and defaults to 1,000.

### Preset references

Astro provides complete evaluators that builders may select instead of repeating or maintaining their definitions. A preset reference is an alternative evaluator-list entry containing only `ref`:

```yaml
schema: evaluation/v1

evaluators:
  - ref: preset/user-sentiment
  - ref: preset/accuracy
```

Each evaluator entry therefore contains either:

- A complete custom definition using the fields described above.
- `ref`, selecting one preset evaluator.

A preset reference accepts no label, configuration, prompt, or output overrides in version 1. A builder who needs different behavior defines a custom evaluator with its own key.

The server validates the reference during publication and resolves its current code-owned definition when needed. Preset definitions are listed below.

### Validation

- `schema` must equal `evaluation/v1`.
- The document must contain 1–10 evaluators.
- Each evaluator entry must be either a supported preset `ref` or a complete custom definition, never both.
- Evaluator keys must be unique after preset references are resolved.
- Custom keys must match `[a-z][a-z0-9_]{0,63}`.
- Labels must contain 1–50 characters.
- Version 1 accepts only `type: llm` for custom evaluators.
- A custom LLM evaluator must contain exactly one of `prompt` or `prompt_file`.
- Inline and resolved prompts must contain 1–8,000 characters.
- Unknown evaluator, configuration, and output fields are rejected.
- Prompt files must exist, use UTF-8 Markdown with the `.md` extension, and resolve from a relative path within the agent project.
- The UTF-8 source YAML plus all distinct prompt files may contain at most 128 KiB in total.
- Each `output` must satisfy its type-specific contract.

### Normalization and references

The parser validates preset references, resolves prompt files, applies defaults to custom evaluators, and normalizes line endings. The normalized definition retains preset references and contains no project paths.

Builder-published evaluation sets use an `evaluation_ref` derived from the ordered normalized definition.

An evaluator within a set is identified by `evaluation_ref` plus its unique `key`. Version 1 does not assign separate per-evaluator references.

Astro-owned definitions use stable references such as:

```text
preset/user-sentiment
preset/default-evaluation
```

Canonical JSON uses UTF-8, sorted object keys, and preserved evaluator and enum-option order. Formatting, comments, YAML key order, and line endings do not affect references. Evaluator order, preset references, and normalized custom evaluator content do affect the evaluation reference.

---

## Preset evaluators

Astro owns preset evaluators and the default-set manifest. They live in a code-owned registry, use stable references, and are not copied into `evaluation_definitions`. Compatible prompt, context, or label changes affect future runs. Existing results are not recomputed or automatically rerun. Breaking output changes require a new preset reference or data migration, and references used by published sets remain resolvable.

### Default evaluation set

An agent without a published evaluation set resolves to Astro's current default set. This section's original design (below, and the `Accuracy`/`Completeness`/`Instruction following` presets that follow) was replaced before shipping — **the real `preset/default-evaluation`** (`internal/evalpreset/set.go`) is six safety/guardrail-oriented presets, not a general quality rubric:

```yaml
ref: preset/default-evaluation

evaluators:
  - ref: preset/exposed-pii              # key: exposed_pii, boolean
  - ref: preset/leaked-credentials       # key: leaked_credentials, boolean
  - ref: preset/disclosed-system-instructions  # key: disclosed_system_instructions, boolean
  - ref: preset/unnecessary-tool-call    # key: unnecessary_tool_call, boolean
  - ref: preset/claim-grounding          # key: claim_grounding, enum: grounded/unsupported/contradicted/no_claims
  - ref: preset/user-sentiment           # key: user_sentiment, enum: positive/neutral/negative/unclear
```

There is also no per-agent custom set: `agent_evaluations`/`EVALUATION.yaml`/publishing don't exist in code (`PostDatasetItem` hardcodes `evalpreset.RefDefaultSet`) — every agent gets this same default. The `accuracy`/`completeness`/`instruction-following` presets documented below this point never shipped; they're kept as the original design record, not as current preset definitions.

### Accuracy

```yaml
ref: preset/accuracy

key: accuracy
label: Accuracy
type: llm
prompt: Determine whether the agent output is factually correct and avoids unsupported or invented claims.
output:
  type: boolean
```

### Completeness

```yaml
ref: preset/completeness

key: completeness
label: Completeness
type: llm
prompt: Determine whether the agent output addresses all material parts of the user's request.
output:
  type: boolean
```

### Instruction following

```yaml
ref: preset/instruction-following

key: instruction_following
label: Instruction following
type: llm
prompt: Determine whether the agent output follows the user's instructions and constraints.
output:
  type: boolean
```

### User sentiment

```yaml
ref: preset/user-sentiment

key: user_sentiment
label: User sentiment
type: llm
config:
  context:
    previous_turns: true
    next_user_message: true
    user_feedback: true
prompt: Determine the user's sentiment toward the agent response. Use conversation context, the next user message, and explicit feedback as evidence.
output:
  type: enum
  options:
    - positive
    - neutral
    - negative
    - unclear
```

---

## Evaluation definitions and agent activation

Builder-published evaluation sets are stored once by immutable reference. `evaluation_definitions` does not store code-owned preset definitions or default-set manifests:

```sql
CREATE TABLE public.evaluation_definitions (
    evaluation_ref  text        NOT NULL,
    definition_json jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evaluation_definitions_pkey PRIMARY KEY (evaluation_ref)
);
```

An agent with a published evaluation set has one mutable pointer to its immutable definition:

```sql
CREATE TABLE public.agent_evaluations (
    account_id     uuid        NOT NULL,
    agent_name     text        NOT NULL,
    evaluation_ref text        NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_evaluations_pkey PRIMARY KEY (account_id, agent_name),
    CONSTRAINT agent_evaluations_agent_fkey FOREIGN KEY (account_id, agent_name) REFERENCES public.agents(account_id, name) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT agent_evaluations_definition_fkey FOREIGN KEY (evaluation_ref) REFERENCES public.evaluation_definitions(evaluation_ref)
);
```

Publishing `EVALUATION.yaml` stores an immutable normalized definition and makes it the agent's active evaluation set. The source YAML is discarded; `definition_json` keeps preset references and embeds resolved prompt-file contents. Agents without a published set use the code-owned default. Changes apply to all deployments of the agent without a rollout. Resolution loads the active set from the preset registry or `evaluation_definitions`, then resolves any preset entries from the registry.

---

## Persistence

The new tables are added without changing legacy writes. The UI cutover writes dataset membership and verified evaluator outputs together. Legacy APIs, tables, and Langfuse judgment data are removed afterward; data already written to the new tables is retained. Existing judgment data is not migrated.

### `eval_datasets`

The dataset record no longer stores verdict counters:

```sql
CREATE TABLE public.eval_datasets (
    id                    uuid        NOT NULL DEFAULT gen_random_uuid(),
    deployment_id         varchar(11) NOT NULL,
    account_id            uuid        NOT NULL,
    langfuse_dataset_name varchar     NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_datasets_pkey PRIMARY KEY (id),
    CONSTRAINT eval_datasets_deployment_id_key UNIQUE (deployment_id),
    CONSTRAINT eval_datasets_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT eval_datasets_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);
```

### `eval_dataset_dismissed_traces`

Dismissal is separate from dataset membership and evaluator results:

```sql
CREATE TABLE public.eval_dataset_dismissed_traces (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_dismissed_traces_pkey PRIMARY KEY (eval_dataset_id, trace_id),
    CONSTRAINT eval_dataset_dismissed_traces_dataset_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE
);
```

A dismissed trace is excluded from the review queue and bulk evaluation actions but is not written to Langfuse. Restoring it deletes the dismissal row. Dataset membership and dismissal are mutually exclusive: adding a trace removes an existing dismissal, and dismissing a dataset item is rejected until the item is removed from the dataset.

### `eval_dataset_evaluation_runs`

One row represents one trace-level job for one evaluation set:

```sql
CREATE TABLE public.eval_dataset_evaluation_runs (
    id                   uuid        NOT NULL DEFAULT gen_random_uuid(),
    eval_dataset_id      uuid        NOT NULL,
    trace_id             text        NOT NULL,
    trace_timestamp      timestamptz NOT NULL,
    evaluation_ref       text        NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    error_message        text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_evaluation_runs_pkey PRIMARY KEY (id),
    CONSTRAINT eval_dataset_evaluation_runs_dataset_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_evaluation_runs_status_check CHECK (status IN ('queued', 'in_progress', 'completed', 'failed'))
);

CREATE UNIQUE INDEX eval_dataset_evaluation_runs_active_idx
    ON public.eval_dataset_evaluation_runs
    (eval_dataset_id, trace_id, evaluation_ref)
    WHERE status IN ('queued', 'in_progress');

CREATE INDEX eval_dataset_evaluation_runs_latest_idx
    ON public.eval_dataset_evaluation_runs
    (eval_dataset_id, trace_id, created_at DESC);
```

Each attempt creates a new run. Terminal runs and their results are immutable so a dataset item can retain the exact automated outputs reviewed for admission. The partial unique index makes requests idempotent while an attempt is active.

### `eval_dataset_evaluator_results`

One row stores one evaluator's result within a run:

```sql
CREATE TABLE public.eval_dataset_evaluator_results (
    evaluation_run_id uuid        NOT NULL,
    evaluator_key     text        NOT NULL,
    status            text        NOT NULL DEFAULT 'queued',
    value_json        jsonb,
    confidence        double precision,
    explanation       text,
    error_message     text,
    CONSTRAINT eval_dataset_evaluator_results_pkey PRIMARY KEY (evaluation_run_id, evaluator_key),
    CONSTRAINT eval_dataset_evaluator_results_run_fkey FOREIGN KEY (evaluation_run_id) REFERENCES public.eval_dataset_evaluation_runs(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_evaluator_results_status_check CHECK (status IN ('queued', 'in_progress', 'completed', 'failed')),
    CONSTRAINT eval_dataset_evaluator_results_confidence_check CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1)
);
```

`value_json` stores the declared boolean, enum string, number, or string value. The worker validates it against the resolved evaluator output schema before marking the result completed. `value_json`, `confidence`, and `explanation` are required for a completed LLM result. Their columns remain nullable because queued, in-progress, and failed results do not contain completed output.

### `eval_dataset_items`

One row records the trace, evaluation run, and method used to add a dataset item. **As shipped**, `source_evaluation_run_id` is nullable, not `NOT NULL` as originally specified — `resolveItemSourceRun` (`handlers/dataset_items.go`) explicitly allows adding an item with no run. The user column is also renamed from what's specified above: it's `verified_by_user_id`, not `added_by_user_id`, matching `internal/evalitemstore`'s `SetVerifiedBy` and every read/write path, and it's nullable rather than `NOT NULL`.

```sql
CREATE TABLE public.eval_dataset_items (
    eval_dataset_id          uuid        NOT NULL,
    trace_id                 text        NOT NULL,
    evaluation_ref           text        NOT NULL,
    source_evaluation_run_id uuid,
    verified_by_user_id      text,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_items_pkey PRIMARY KEY (eval_dataset_id, trace_id),
    CONSTRAINT eval_dataset_items_dataset_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_items_run_fkey FOREIGN KEY (source_evaluation_run_id) REFERENCES public.eval_dataset_evaluation_runs(id)
);
```

### `eval_dataset_item_evaluator_outputs`

One row stores a dataset item's final value for one evaluator. **As shipped**, this table has only four columns — `value_source` and `verified_by_user_id` (and the provenance tracking they imply) were never built, along with `created_at`/`updated_at`:

```sql
CREATE TABLE public.eval_dataset_item_evaluator_outputs (
    eval_dataset_id     uuid        NOT NULL,
    trace_id            text        NOT NULL,
    evaluator_key       text        NOT NULL,
    value_json          jsonb       NOT NULL,
    CONSTRAINT eval_dataset_item_evaluator_outputs_pkey PRIMARY KEY (eval_dataset_id, trace_id, evaluator_key),
    CONSTRAINT eval_dataset_item_evaluator_outputs_item_fkey FOREIGN KEY (eval_dataset_id, trace_id) REFERENCES public.eval_dataset_items(eval_dataset_id, trace_id) ON DELETE CASCADE
);
```

The paragraph below describes the original design's provenance tracking (`value_source`, `verified_by_user_id`) — none of it is implemented. There's no way today to tell whether a stored value was accepted automated output or a reviewer's override.

~~`value_source` records whether the final value was copied from the referenced run or supplied or changed by a reviewer. `verified_by_user_id` is set on every output, including accepted automated values. The server validates every value against the item's evaluation set; automated results remain unchanged.~~

### Removed tables and fields

The following tables are removed:

```text
eval_dataset_judgments
eval_dataset_judgment_reasons
eval_dataset_judgment_predictions
eval_dataset_judgment_prediction_criteria
eval_dataset_prediction_requests
```

The following concepts are not carried into the replacement schema:

- `good_count` and `bad_count`.
- Human or model verdicts.
- Verdict scores and thresholds.
- Judgment criteria and reasons.
- Verdict-derived dataset membership.

---

## LLM evaluator execution

### Input

Each LLM call receives system-owned instructions, one resolved evaluator, and the trace context allowed by that evaluator:

```json
{
  "evaluator": {
    "key": "user_sentiment",
    "prompt": "Determine the user's sentiment toward the agent response.",
    "output": {
      "type": "enum",
      "options": ["positive", "neutral", "negative", "unclear"]
    }
  },
  "trace": {
    "trace_id": "trace_123",
    "input": "<trace-input>",
    "output": "<trace-output>"
  },
  "previous_turns": [],
  "next_user_message": "<next-message>",
  "user_feedback": "thumbs_down"
}
```

Fields disabled by context configuration are omitted. All values are serialized with a JSON encoder. Evaluator prompts are rubric content and cannot change system instructions or the required output schema.

### Structured response

The server generates a strict response schema from the declared output:

```json
{
  "value": "negative",
  "confidence": 0.88,
  "explanation": "The next user message expresses dissatisfaction with the response."
}
```

- `value` must match the evaluator's declared output schema.
- `confidence` is a number from 0–1.
- `explanation` is one concise sentence and is limited to 240 characters after validation.
- Unknown fields are rejected.

All three fields are required. `confidence` and `explanation` are standard LLM evaluator metadata owned by Astro and are not configurable in `EVALUATION.yaml`.

### Trace-level job

One queue job evaluates one trace:

```text
Trace evaluation job
├── Run evaluator 1 with its own LLM call
├── Store evaluator 1 result
├── Run evaluator 2 with its own LLM call
├── Store evaluator 2 result
└── Finalize the trace run
```

Version 1 behavior:

- A bulk action creates one run and enqueues one job per eligible trace.
- The job records the active `evaluation_ref`. It loads the set from the preset registry or `evaluation_definitions`, then resolves any preset entries from the registry.
- One result row is created for every evaluator in the resolved set.
- Evaluators run sequentially in definition order.
- Every evaluator makes a separate model call.
- Successful evaluator results are committed independently and survive sibling failures.
- Failed or invalid evaluator calls follow the existing bounded retry policy before being stored as failed.
- Multiple evaluators are not batched into one model request.

The run finishes as:

- `completed` when every evaluator attempt has finished, including when individual evaluators failed.
- `failed` when a trace-level error prevents evaluator execution, such as failing to load the trace or evaluation definition.

---

## API

### Agent registration

```http
POST /api/v1/agents/:account/:name/register
```

The existing agent registration request accepts an optional `evaluation` object containing the complete evaluation document and any referenced prompt-file contents:

```json
{
  "build_id": "a3f2b1c9",
  "spec_content": "<transformed astropods.yml>",
  "evaluation": {
    "evaluation_yaml": "schema: evaluation/v1\nevaluators:\n  - key: response_quality\n    label: Response quality\n    type: llm\n    prompt_file: evaluation/response-quality.md\n    output:\n      type: number\n      minimum: 0\n      maximum: 1\n",
    "prompt_files": {
      "evaluation/response-quality.md": "Assess the overall quality of the agent response."
    }
  }
}
```

`evaluation_yaml` contains the complete authored document. `prompt_files` maps each referenced project-relative path to its UTF-8 contents and is empty when every evaluator uses an inline prompt or preset. Missing referenced files, unreferenced file entries, and paths outside the project are rejected.

An evaluation-aware CLI always includes the field:

- It sends an `evaluation` object when `EVALUATION.yaml` exists.
- It sends `"evaluation": null` when the file is absent, restoring the code-owned default.

Only older clients that do not support evaluation configuration omit the field. The server treats omission as no change.

The server validates a supplied set before atomically storing the registration and updating `agent_evaluations`. Invalid content changes neither. When only `EVALUATION.yaml` changes, `ast push` reuses the current build registration and skips the container build and push.

### Review queue

```http
GET /api/v1/deployments/:id/dataset/review-queue
```

Preserves the existing per-trace review-queue structure, replacing prediction fields with evaluation fields and replacing prediction criteria with evaluator outputs. It returns traces that are neither dataset items nor dismissed. The existing `prediction` query filter is replaced with:

```text
evaluation=evaluated|not_evaluated
```

`evaluated` includes traces with a completed run containing at least one completed evaluator result, regardless of evaluation reference. `not_evaluated` includes traces without one. Omitting the filter returns both.

The `evaluated` filter pages distinct trace IDs from completed runs with a successful result locally by trace timestamp, then fetches only those traces from Langfuse. The unfiltered and `not_evaluated` paths scan timestamp-ordered Langfuse traces and batch-load their local state.

**As shipped, the list response below is much thinner** — per-item `output` and the embedded `evaluators[]` array never made it into the list endpoint (fetching full evaluator detail for every paginated item would be expensive). The real list item is just `{trace_id, timestamp, input, run: {status, error}}` alongside `next_cursor`. Everything this example shows under `evaluation.evaluators` — `output`, `result.confidence`, `result.explanation`, per-evaluator `status` — is real, but only on the separate per-trace detail endpoint, `GET .../review-queue/:trace_id/evaluation`:

```json
{
  "items": [
    {
      "trace_id": "trace_123",
      "timestamp": "2026-08-12T14:00:00Z",
      "input": "<trace-input>",
      "output": "<trace-output>",
      "evaluation": {
        "evaluation_ref": "preset/default-evaluation",
        "run": {
          "status": "completed",
          "error": null
        },
        "evaluators": [
          {
            "key": "accuracy",
            "label": "Accuracy",
            "type": "llm",
            "output": { "type": "boolean" },
            "result": {
              "status": "completed",
              "value": true,
              "confidence": 0.94,
              "explanation": "The response is consistent with the available evidence."
            }
          },
          {
            "key": "user_sentiment",
            "label": "User sentiment",
            "type": "llm",
            "output": {
              "type": "enum",
              "options": ["positive", "neutral", "negative", "unclear"]
            },
            "result": {
              "status": "failed",
              "error": "Expected one configured enum value."
            }
          }
        ]
      }
    }
  ],
  "next_cursor": "<cursor>"
}
```

`evaluation` always describes the agent's active set, including when `run` and every `result` are null. It returns evaluator results only for the active `evaluation_ref`. Results under older references still satisfy the Evaluated filter but are not shown as current values.

Run evaluators for the next eligible traces:

```http
POST /api/v1/deployments/:id/dataset/evaluations
```

This was framed as renaming an existing `POST /api/v1/deployments/:id/dataset/predictions` endpoint, but that endpoint was never actually reachable — no route was ever registered for it, and the judge-signal worker it would have driven (`EvalJudgePredictionWorker`) has no production caller anywhere. `POST .../dataset/evaluations` is what shipped, as a new endpoint, not a rename. It accepts no request body and queues one trace-level evaluation run for each of the most recent eligible review-queue traces, up to the existing limit. Traces already in the dataset or dismissed from the queue are ineligible.

The response reports the enqueued and failed trace IDs. Re-requesting an active run is idempotent. Eligible traces have no completed evaluator result for the active `evaluation_ref`. A trace evaluated under an older reference remains Evaluated but is eligible to run against the active set. Code-owned preset and default-manifest updates do not make completed traces eligible again.

Read aggregate execution status:

```http
GET /api/v1/deployments/:id/dataset/evaluations/status
```

This renames the existing prediction-status endpoint and returns queued, in-progress, completed, and failed run counts for frontend polling.

Dismiss a trace without adding it to the dataset:

```http
POST /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
```

Restore a dismissed trace to the queue:

```http
DELETE /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
```

These mutations only update `eval_dataset_dismissed_traces`. They do not modify Langfuse dataset items or evaluator results. Both are idempotent and return 200; dismissing a dataset item returns 409.

---

### Dataset summary

```http
GET /api/v1/datasets/:id
```

Real, shipped path: `GET /api/v1/deployments/:id/dataset`.

```json
{
  "id": "f5eb29f4-4778-4bb5-9941-cb35cb90e04e",
  "dataset_name": "eval-dep_123",
  "item_count": 40,
  "evaluation_ref": "preset/default-evaluation",
  "evaluators": [
    {
      "key": "accuracy",
      "label": "Accuracy",
      "type": "llm",
      "output": { "type": "boolean" },
      "distribution": [
        { "value": true, "count": 24, "percentage": 0.75 },
        { "value": false, "count": 8, "percentage": 0.25 }
      ]
    },
    {
      "key": "user_sentiment",
      "label": "User sentiment",
      "type": "llm",
      "output": {
        "type": "enum",
        "options": ["positive", "neutral", "negative", "unclear"]
      },
      "distribution": [
        { "value": "positive", "count": 12, "percentage": 0.4 },
        { "value": "neutral", "count": 9, "percentage": 0.3 },
        { "value": "negative", "count": 7, "percentage": 0.2333 },
        { "value": "unclear", "count": 2, "percentage": 0.0667 }
      ]
    }
  ]
}
```

### Dataset items

List dataset items:

```http
GET /api/v1/datasets/:id/items
```

Real, shipped path: `GET /api/v1/deployments/:id/dataset/items`.

The response preserves the existing paginated Langfuse item and trace fields. Each item also includes its evaluation schema, final evaluator outputs, and the automated results from its source run.

The dataset-item mutations rework the existing judgment endpoints rather than adding a parallel workflow. Adding an item replaces `POST /api/v1/deployments/:id/dataset/judgments`, moves dataset identity into the path, and removes the verdict from its request:

```http
POST /api/v1/datasets/:id/items
```

Real, shipped path: `POST /api/v1/deployments/:id/dataset/items`.

```json
{
  "trace_id": "trace_123",
  "evaluation_run_id": "6f1599a0-d1d4-47eb-9c47-30e10ab81e80",
  "evaluator_outputs": [
    { "key": "accuracy", "value": true },
    { "key": "user_sentiment", "value": "negative" }
  ]
}
```

The run must be completed, belong to the trace and dataset, and use the active evaluation set. The request must provide a valid final value for every evaluator, including evaluators whose automated calls failed. The server derives `value_source` by comparing each submitted value with the source result and records the reviewer on every output.

Creating the membership row and evaluator-output rows is one local transaction. The existing deterministic Langfuse identity, duplicate protection, and compensation behavior then create the corresponding Langfuse dataset item with the final evaluator outputs. A successful add removes the trace from the review queue.

Removing it uses:

```http
DELETE /api/v1/datasets/:id/items/:trace_id
```

Real, shipped path: `DELETE /api/v1/deployments/:id/dataset/items/:trace_id`.

Removing the item deletes its Langfuse item and cascades to its final evaluator outputs. Automated runs and results remain available, so the trace returns to the review queue unless dismissed.

Edit a dataset item's final outputs:

```http
PUT /api/v1/datasets/:id/items/:trace_id/evaluator-outputs
```

Real, shipped path: `PUT /api/v1/deployments/:id/dataset/items/:trace_id/evaluator-outputs`.

```json
{
  "values": [
    { "key": "accuracy", "value": false },
    { "key": "user_sentiment", "value": "negative" }
  ]
}
```

The request replaces every final output using the evaluation set stored on the item. Values matching completed source results retain automated provenance; changed or supplied values use human provenance. Every output records the current reviewer, and `updated_at` records when it was last verified. Automated results remain unchanged.

---

## Frontend

### Review queue trace detail

Each review-queue trace continues showing its input and output. The predicted-verdict area is replaced with:

- Trace-level evaluation status.
- One row or compact value for every evaluator in definition order.
- The evaluator's typed value.
- Confidence and explanation when available.
- Per-evaluator loading or failure state.

Evaluator values use type-appropriate presentation, such as yes/no for booleans, labels for enums, formatted numbers, and text for strings.

### Review actions

The per-trace review actions are:

- Verify the evaluator outputs and add the trace to the dataset.
- Remove the trace from the review queue without adding it.

Run evaluators remains a queue-level action. Agree, disagree, and human-verdict controls are removed. The good/bad filter is replaced with Evaluated and Not evaluated options using the review-queue definition above.

### Evaluator output verification

The current criteria controls are replaced with a type-appropriate output control for each evaluator in definition order. Completed automated values initialize the controls; failed evaluators require a human value. Edits remain local until **Add to dataset** stores the item and final outputs together. Automated results remain unchanged.

### Dataset grade

The existing dataset grade and good/bad breakdown are replaced with one distribution for each evaluator in the agent's active evaluation set, calculated from `eval_dataset_item_evaluator_outputs`.

- Boolean evaluators show counts and percentages for `true` and `false`.
- Enum evaluators show counts and percentages for each configured option in definition order.
- Number evaluators show a histogram across the configured minimum and maximum.
- String evaluators show the number of stored values but no value distribution because their values are unbounded.

Counts and percentages are calculated only from evaluator values that exist. The distributions do not report missing or failed values, produce an overall grade, rank values, or imply that one distribution is better than another.

### Dataset item edit

The dataset item list continues showing traces added by reviewers. Its editor compares each final evaluator output with the automated result from the source run and allows the final value to be changed.

The item menu also retains removal from the dataset. Removing an item deletes its corresponding Langfuse item and final evaluator outputs but preserves evaluator runs and results.

---

## Future evaluator types

`type` is a discriminator. Each future evaluator type may define its own configuration, execution engine, and validation while using the same common identity, output, run, result, and API contracts.

Conceptual future definitions include:

```yaml
- key: purpose
  label: Conversation purpose
  type: classifier
  config:
    classifier: astro-work-classifier/v1
  output:
    type: enum
    options: [work, personal]
```

```yaml
- key: follows_policy
  label: Follows policy
  type: code
  config:
    entrypoint: evaluation/follows-policy.ts
  output:
    type: boolean
```

These shapes are illustrative and are rejected by `evaluation/v1` until their contracts and engines are specified.

---

## Rollout

Merge the rollout in this order:

1. **Dataset actions:** Remove verdict controls and presentation. Use hidden legacy `good` judgments to add traces to the dataset and `unknown` judgments to remove traces from the queue. Replace the good/bad grade with distributions of the existing criteria values.
2. **Neutral evaluation UI:** Reframe the existing criteria modal as **Evaluate trace** while retaining its current controls, APIs, and storage.
3. **Evaluation foundation:** Add the preset registry and the run, result, dataset-item output, membership, and dismissal tables.
4. **Preset evaluation backend:** Run the default preset evaluators for selected traces, store their results, and expose execution progress and results to the review queue.
5. **Dataset review APIs:** Add item admission, output editing, dismissal, filtering, and distribution APIs backed by the new tables.
6. **Evaluation UI cutover:** Connect the existing evaluation presentation and **Evaluate trace** controls to preset evaluator results and the new dataset APIs; update filters, distributions, and dataset-item editing.
7. **Legacy cleanup:** Remove unused judgment APIs, workers, tables, and Langfuse data.
8. **Custom evaluation backend:** Accept and activate custom sets during agent registration and execute their preset and custom evaluators.
9. **Custom evaluation CLI:** Add `EVALUATION.yaml` discovery and validation to `ast push`.
