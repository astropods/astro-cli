# Build and deployment orchestration

`agent redeploy --commit <full-sha> --wait --json` waits for publication, selects that build, and waits for its deployment revision.
`blueprint deploy` accepts the same flags for a new deployment.
`--timeout` defaults to 45 minutes and covers authentication, target resolution, publication, submission, and rollout together.
AgentCore deployments select artifacts with `--image` and use their own readiness timeout.
They reject `--commit`, `--build`, and `--timeout` before deployment (see `cmd/agentcore_test.go`).

```sh
ast blueprint builds wait my-blueprint --commit "$COMMIT_SHA" --json
ast agent redeploy --id "$DEPLOYMENT_ID" --commit "$COMMIT_SHA" --wait --json
ast agent wait --id "$DEPLOYMENT_ID" --build "$BUILD_ID" --revision 2 --json
```

The build waiter sends exact selectors to the server, then verifies the returned commit and build.
It pins the first matching attempt. Failed, canceled, and skipped attempts return errors.
Commit selection accepts only clean CLI provenance or server builds.
An explicit build ID can select a local artifact with unknown provenance.

Redeployment preserves existing adapters, variables, schedules, and other stored configuration when their flags are omitted.
The CLI reads the target and revision together before waiting, then requests a template that preserves stored configuration.
Default redeployment preserves the current revision's build and configuration after rollback.
It supplies that revision as `If-Match` when submitting the template.
Plain redeploys also send the reported revision, including requests without `--commit` or `--wait`.
Older servers that omit revisions still support plain redeploys; orchestration requires revision support.
Redeployment preserves the display name returned by the server template, including a concurrent rename.
`TestRedeployPreservesTheTemplateNameAfterConcurrentConfigurationChanges` covers this behavior.
A concurrent change fails the precondition. A newer revision detected during waiting fails the wait.
The revision is read before the build wait, so another change during a long wait fails the precondition at submission. That keeps omitted flags faithful to the configuration that was read, and re-running is cheap because the build is already published.
`--commit` resolves a build from the publisher's history, which a caller who cannot edit the blueprint cannot read. Deploy a published build of someone else's blueprint with `--build`.

Readiness requires both the expected current revision and the server's matching `ready_revision` observation.
A missing chat URL does not imply success. A deadline or cancellation returns an error.
A deployment the server reports as `failed` or `error` ends the wait with its message, because the server sets those only after its own retries are exhausted.
Unchanged `OnDelete` dependencies retain their pods. A changed dependency template is rolled out by the server, which replaces the stale pods of a `recreate` dependency, so waiting needs no operator step.

`--json` writes one result to stdout after the requested wait. Progress goes to stderr.
A successful wait without `--json` prints the launch URL from the record the readiness check already read.
`--dry-run --json` prints the validated template without its signature, which authorizes a deploy the dry run never submits.
MCP hides `--timeout` on the waiting commands, because the runner ends the subprocess at its own tool timeout.
Timeout errors include the deployment, build, and revision needed by `agent wait`.
Waiting does not cancel a build or deployment, and resuming a wait does not submit another deployment.

Polling reads inside a wait retry transient failures up to three times per read.
Backoff starts at 250 milliseconds, doubles per retry, and adds jitter up to the current delay.
The reader honors `Retry-After` up to thirty seconds, and cancellation or the shared timeout interrupts backoff.
Resolving a `--id` or `--env` target reads once. That read is shared with commands that set no deadline, so it never retries, including in the commands that do set one.
`TestTargetReadsDoNotRetryAndRetryAfterIsCapped` covers both halves, and `TestRedeployTimeoutBoundsTheBuildWait` covers the deadline.
Retryable HTTP statuses are `408`, `429`, `500`, `502`, `503`, and `504` (`cmd/polling_read.go`).
Authentication failures, missing resources, revision conflicts, and malformed JSON fail immediately.
Template and deployment POSTs remain single attempts (`TestDeployDoesNotRetryFailedWrites`).

For CI, set `ASTRO_ACCOUNT` and `ASTRO_ACCESS_TOKEN`. These bypass saved-profile lookup together.
Machine tokens retain their server-side account binding and permission scopes.
The CLI cannot renew client-credentials tokens. Obtain a new token and resume the wait after expiry.

The server must support build selectors, deployment revision preconditions, and revision readiness observations.
The command tests in `cmd/orchestration_test.go` verify the HTTP contracts and stdout behavior.
The server repository contains the local kind fixture and a GitHub Actions example.
