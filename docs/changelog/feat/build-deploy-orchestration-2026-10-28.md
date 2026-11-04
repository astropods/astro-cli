# Build and deployment orchestration

## Summary

CI selects one commit's published blueprint build and waits for that deployment revision to become ready.
The commands support account-bound machine tokens without a saved human profile.

## Design

`blueprint builds wait` queries exact commit or build selectors and pins the selected attempt.
`agent redeploy --commit SHA --wait` preserves omitted configuration and submits the deployment's revision as an `If-Match` precondition.
The target and revision come from one deployment record read before requesting a template that preserves stored configuration.
Plain redeploys use the same read and send the revision when available, covering requests without build selection or waiting.
Orchestration options remain separate from the HTTP template payload, and deployment errors use the shared message functions.
Target resolution shares ID, name, and environment lookup without rereading the full record before submission.
Deployment reads preserve authorization and availability errors; only missing deployments receive the not-found message.
Redeployment preserves the server template's display name, avoiding stale name writes during concurrent changes.
The shared deadline starts before authentication and target resolution, covering publication, submission, and rollout.
Readiness requires the expected build, revision, and server readiness observation.
Only the waiting commands retry. Build, deployment-detail, and runtime reads inside a wait retry transient network errors and selected HTTP failures up to three times per read.
Exponential backoff adds jitter, stops on cancellation or the command deadline, and honors `Retry-After` up to thirty seconds.
Resolving a `--id` or `--env` target reads once, because those commands set no deadline to bound a retry loop against.
Authentication errors, missing resources, revision conflicts, and invalid JSON fail immediately.
Writes remain single attempts so an uncertain response cannot submit another deployment.
Regression coverage exercises recovery across all polling reads, retry exhaustion, cancellation, and failed writes.
Push tests reset both command forms, preventing flag state from leaking across repeated suite runs.

AgentCore rejects unsupported commit, build, and timeout flags before deployment, preventing silent artifact substitution.
Machine and saved-profile authentication share token validation and expiry handling after account selection.
A machine token stands in for a saved login only when `ASTRO_ACCOUNT` names the account it was issued for. Setting the token alone fails with that instruction rather than authenticating a saved org account with an unrelated credential.

JSON output contains one result after the requested wait; progress goes to stderr.
`agent wait` resumes observation after interruption without submitting another deployment.
A server that reports no deployment revision downgrades `--wait` to a warning, leaving a submitted deployment reported as deployed rather than failing a command that already created it.
A stale `If-Match` and an in-progress rollout each report what to do next instead of surfacing the raw status.


## Migration

Install the server's build-selector and revision-readiness support before using these commands.
`--wait` now checks workload readiness instead of the public chat URL.
Set `ASTRO_ACCOUNT` and `ASTRO_ACCESS_TOKEN` for headless use.
Obtain a new machine token after expiry and resume with `agent wait`.
