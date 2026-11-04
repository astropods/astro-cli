# Automatic deployment commands

## Summary

Operators can prepare, inspect, and recover evaluation-gated deployment workflows from the CLI.

## Design

Setup validates a fixed dataset and the candidate's active evaluator set, then saves a pinned acceptance plan.
Applying enables the plan that was reviewed. It refuses when the target, candidate, acceptance version, build, or revision differs from the saved plan, and leaves that file in place so the reviewed plan survives the refusal. The version is a hash of the acceptance definition and carries neither deployment, so both are compared on their own. The plan file is evidence of a review rather than an input, because applying recomputes the policy from the dataset, rules, and runner. A stored version that still disagrees with the reviewed plan is reported with both versions, which tells the operator a write already happened.
Applying checks permissions before configuring the candidate and target. It preserves the evaluator configuration and requires a separate trusted runner.

Status and paginated history expose publication, evaluation, acceptance, and readiness progress.
Retry uses an expected build, policy version, and stable request ID to recover blocked publications without repeating evaluation calls.
A resume reads the current policy even when a policy version is supplied, which is how it recognizes its own request in the version the retry stored. Finding that request already applied succeeds and reports the policy, because recovering a lost response achieved what the operator asked for. A request that landed for a different build is refused instead, so reusing a request ID with the wrong build never reads as success. A deployment with no policy version is named directly, instead of rejecting a policy version the operator never supplied.
A rejected retry returns the server's reason. The hint to resume is limited to a lost, throttled, or unreadable response, which are the cases where the request may have landed and repeating it helps.
Status reports a target as ready only for the submitted build and revision, discarding a server flag that describes a publication the target has moved past.
Dataset snapshots export the evaluator values a case carries in the dataset, which are the values a human review confirmed when one exists and the values an evaluation run produced otherwise. Acceptance rules gate on them either way, so the help text and the guide ask for a reviewed dataset rather than claiming the export verifies anything. Build comparison reports changes against matching historical evidence.
Enabling automatic deployment discards the response body, because the write is the outcome and an empty success would otherwise read as a partial configuration.

## Migration

Use a server with the release workflow endpoints. See `docs/release-workflows.md` for commands and prerequisites.
