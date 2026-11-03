# Automatic deployment workflows

These commands require a server with build evaluation and promotion support.

## Prepare a workflow

Use separate candidate and target deployments of the same blueprint in the same account.
Configure the candidate's active evaluators for input/output evaluation first.

Export reviewed dataset items, including their expected outputs and verified evaluator values:

```sh
ast dataset snapshot regression-cases --output dataset.json
```

Snapshots contain 1 to 50 cases. For larger datasets, select a batch explicitly with `--limit` and `--offset`.
Review outdated evaluator values before exporting.

Write an acceptance rules array, for example:

```json
[{"key":"accuracy","matches_expected":true,"min_confidence":0.8}]
```

Validate the dataset and active evaluator configuration and save a plan:

```sh
ast agent release setup --id TARGET --candidate CANDIDATE \
  --dataset dataset.json --rules rules.json --runner app:RUNNER_ID
```

Review `promotion-plan.json`, then repeat with `--apply` to enable future publications.
Setup checks permissions before writing policies. It does not create a runner or change the active evaluator set.
Configure the trusted runner with this dataset and policy version before publishing.
The server repository's build evaluation workflow guide covers runner artifacts, the reference adapter, and scheduling.

Applying configures the candidate before the target. If target configuration fails, inspect both deployments and repeat setup after resolving the cause.

## Inspect and recover

```sh
ast agent release status --id TARGET
ast agent release history --id CANDIDATE --limit 20 --json
ast agent release history --id CANDIDATE --policies --json
ast agent release retry --id TARGET --build BUILD_ID
```

Status reports the current publication, evaluation progress, and submitted revision readiness.
History returns `next_cursor` for subsequent requests with `--cursor`. Each run includes its latest acceptance decision and policy version.
That decision can use a different policy from the target's pinned policy.

Retry requires a blocked or failed publication and rechecks current permissions and deployment gates.
It preserves the policy owner and does not repeat candidate calls or evaluator calls.
If a retry response is lost, reuse the printed `--policy-version` and `--request-id` to recover the same request.
A superseded publication or changed policy returns a conflict.

## Compare builds

```sh
ast agent release compare --id CANDIDATE --run CURRENT_RUN --baseline PREVIOUS_RUN --json
```

Comparison requires completed runs from different builds with identical datasets and evaluator configuration.
The earlier run is the baseline. Changes in verified-value matches count as improvements or regressions.
Numeric values include deltas. Comparison does not change acceptance or trigger deployment.
