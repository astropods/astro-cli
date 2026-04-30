## Summary

The `TestDeploy_WithDeploymentID_UpdatesExisting` test was failing because it expected a `DELETE FROM deployment_variables` query inside the `UpdateDeploymentPending` transaction, but the table was renamed to `deployment_build_env` in a prior migration. The mock expectation was never updated to match.

## Design

`UpdateDeploymentPending` in `deploymentstore/store.go` clears three normalized tables before re-inserting: `deployment_workloads`, `deployment_sidecars`, and `deployment_build_env`. The test's sqlmock expectation for the third delete still referenced the old table name, causing sqlmock to reject the actual query and return a 500.

The fix updates the single expectation from `DELETE FROM deployment_variables` to `DELETE FROM deployment_build_env`, matching what the store now executes.

## Migration

No migration required.
