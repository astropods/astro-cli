# Cluster deploy validation (M0 Phase 1)

## Summary

Hardens multi-region deploy and undeploy on the server: deploys to unhealthy clusters are rejected before creating a row, and undeploy completes DB cleanup only when cluster-client resolution fails permanently (IAM deny, deregistered, or disabled cluster)—transient AWS/network errors still fail the job for River retry.

## Design

- **Deploy health check:** After enabled/disabled and unknown-id checks, `clusterHealthForDeploy` calls `k8sReg.Get` + `CheckHealth()` and returns 400 `cluster is unhealthy` before `prepareDeployment`. Raw errors are logged server-side; API `details` use `k8s.PublicClusterHealthDetail` only.
- **Undeploy fail-open (narrow):** `k8s.IsPermanentClientResolutionError` classifies missing/disabled clusters and IAM denies; only those wrap `ErrClusterClientUnavailable` and let `UndeployWorker` skip K8s teardown while marking `undeployed`. Throttling and other transient errors stay fail-closed with River retries.

## Migration

No change for primary-cluster deploys. Operators register clusters via Queen; deploys get synchronous 400s for bad cluster targets instead of async error badges. Cluster placement for accounts is a follow-up (Queen assigns cluster per account; deployment-template generation injects `target.cluster_id` server-side).
