# Automatic deployment commands

## Summary

Operators can prepare, inspect, and recover evaluation-gated deployment workflows from the CLI.

## Design

Setup validates a fixed dataset and the candidate's active evaluator set, then saves a pinned acceptance plan.
Applying checks permissions before configuring the candidate and target. It preserves the evaluator configuration and requires a separate trusted runner.

Status and paginated history expose publication, evaluation, acceptance, and readiness progress.
Retry uses an expected build, policy version, and stable request ID to recover blocked publications without repeating evaluation calls.
Dataset snapshots retain reviewed reference outputs and evaluator values. Build comparison reports changes against matching historical evidence.

## Migration

Use a server with the release workflow endpoints. See `docs/release-workflows.md` for commands and prerequisites.
