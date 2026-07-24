# Eval judge key provisioning foundation

## Summary

Adds the account-scoped judge key that Astro-managed eval dataset judging will use.

## Design

Each account receives one reusable, encrypted judge key under its existing Bifrost customer and budget. Concurrent first use converges on the winning stored key and cleans up redundant mints. An unresolved upstream-only name conflict fails with actionable account and key identity without deleting a potentially in-progress key. The key keeps wildcard model access, so invocation-level model changes need no rotation. Account soft deletion attempts immediate revocation, with final purge providing a best-effort retry without blocking account cleanup. Model invocation, prompting, prediction orchestration, and billing remain separate concerns.

## Migration

No user action is required. The schema change is additive and uses existing gateway configuration.
