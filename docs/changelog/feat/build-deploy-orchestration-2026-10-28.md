# Build and deployment orchestration

## Summary

CI selects one commit's published blueprint build and waits for that deployment revision to become ready.
The commands support account-bound machine tokens without a saved human profile.

## Design

`blueprint builds wait` queries exact commit or build selectors and pins the selected attempt.
`agent redeploy --commit SHA --wait` preserves omitted configuration and submits the deployment's revision as an `If-Match` precondition.
The shared timeout covers publication and rollout. Readiness requires the expected build, revision, and server readiness observation.

JSON output contains one result after the requested wait; progress goes to stderr.
`agent wait` resumes observation after interruption without submitting another deployment.

## Migration

Install the server's build-selector and revision-readiness support before using these commands.
`--wait` now checks workload readiness instead of the public chat URL.
Set `ASTRO_ACCOUNT` and `ASTRO_ACCESS_TOKEN` for headless use.
Obtain a new machine token after expiry and resume with `agent wait`.
