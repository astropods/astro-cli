# Automatic deployment commands

## Summary

Operators can prepare, inspect, and recover evaluation-gated deployment workflows from the CLI.

## Design

Setup validates a fixed dataset and the candidate's active evaluator set, then saves a pinned acceptance plan.
Applying enables the plan that was reviewed. It refuses when the target, candidate, acceptance version, build, or revision differs from the saved plan, and leaves that file in place so the reviewed plan survives the refusal. The version is a hash of the acceptance definition and carries neither deployment, so both are compared on their own. The plan file is evidence of a review rather than an input, because applying recomputes the policy from the dataset, rules, and runner. A stored version that still disagrees with the reviewed plan is reported with both versions, which tells the operator a write already happened.
Applying checks permissions before configuring the candidate and target. It preserves the evaluator configuration and requires a separate trusted runner.

Status and paginated history expose publication, evaluation, acceptance, and readiness progress.
Retry uses an expected build, policy version, and stable request ID to recover blocked publications without repeating evaluation calls.
A resume reads the current policy even when a policy version is supplied, which is how it recognizes its own request in the version the retry stored. Finding that request already applied succeeds and reports the policy, because recovering a lost response achieved what the operator asked for. A deployment with no policy version is named directly, instead of rejecting a policy version the operator never supplied.
A rejected retry returns the server's reason. Only a lost or throttled response carries the hint to resume, which is the case where repeating the request helps.
Status reports a target as ready only for the submitted build and revision, discarding a server flag that describes a publication the target has moved past.
Dataset snapshots retain reviewed reference outputs and evaluator values. Build comparison reports changes against matching historical evidence.

## Migration

Use a server with the release workflow endpoints. See `docs/release-workflows.md` for commands and prerequisites.
