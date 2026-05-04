## Summary

Temporarily disables the preview smoke-test job in the deploy-to-preview workflow. The `astro-testbot` account receives a persistent HTML 403 from `astro-registry` during the push step; retries do not help because the error is not transient. Disabling the job prevents every preview deploy from failing until the registry membership issue is investigated and resolved.

## Design

Added `if: false` to the `smoke-test` job in `deploy-preview.yml` with a TODO comment. The job definition and its `with` block are left intact so the job can be re-enabled by removing the override once the root cause is fixed.

## Migration

No action required.
