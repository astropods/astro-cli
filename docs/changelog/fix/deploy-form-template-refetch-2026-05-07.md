# fix/deploy-form-template-refetch — 2026-05-07

## Summary

Configure / redeploy could keep an old interactive template response while `deployment_id`, pinned `build`, or `revision` changed (for example after clearing upgrade or rollback query params on the same page). Finalize + deploy then mixed stale schema with new pins, which could yield bad specs and unhealthy rollouts.

## Design

- **`useDeployForm`** now keys the bootstrap `POST` template request on a composite identity: account, agent name, optional `deployment_id`, `build`, and `revision`.
- When that key changes, the hook clears the cached template, resets the one-shot seed flag, and refetches—matching what already happens on a full remount (e.g. navigating from Deployments).
- **`reshapeTemplate`** and the finalize **`deploy()`** request now forward **`revision`** when it is set (including `0`), so rollback flows stay pinned during reshapes and submit.

## Migration

None. Users only see correct template reloads when configure context changes; no API or config changes required.
