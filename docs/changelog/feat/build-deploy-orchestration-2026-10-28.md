# Build and deployment orchestration

## Summary

CI selects one commit's published blueprint build and waits for that deployment revision to become ready.
The commands support account-bound machine tokens without a saved human profile.

## Design

`blueprint builds wait` queries exact commit or build selectors and pins the selected attempt.
`agent redeploy --commit SHA --wait` preserves omitted configuration and submits the deployment's revision as an `If-Match` precondition.
The precondition and configuration come from one full deployment record read.
Target resolution shares ID, name, and environment lookup without rereading the full record before submission.
Deployment reads preserve authorization and availability errors; only missing deployments receive the not-found message.
Redeployment preserves the server template's display name, avoiding stale name writes during concurrent changes.
The shared timeout covers publication and rollout. Readiness requires the expected build, revision, and server readiness observation.

AgentCore rejects unsupported commit, build, and timeout flags before deployment, preventing silent artifact substitution.
Machine and saved-profile authentication share token validation and expiry handling after account selection.

JSON output contains one result after the requested wait; progress goes to stderr.
`agent wait` resumes observation after interruption without submitting another deployment.

## Migration

Install the server's build-selector and revision-readiness support before using these commands.
`--wait` now checks workload readiness instead of the public chat URL.
Set `ASTRO_ACCOUNT` and `ASTRO_ACCESS_TOKEN` for headless use.
Obtain a new machine token after expiry and resume with `agent wait`.
