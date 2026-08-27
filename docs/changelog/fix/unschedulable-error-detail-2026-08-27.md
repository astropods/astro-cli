# Log why a deployment could not be scheduled

## Summary

`deploycontroller: deployment failed … reason=Unschedulable` was the only thing
the server said when the scheduler refused a pod. Kubernetes uses that one
reason string for every scheduling failure, so the line could not distinguish a
pod whose node selector matches nothing from a pod that simply does not fit.
Diagnosing either meant leaving the logs and running `kubectl describe pod`.

## Design

The controller already had the answer. `aggregateDeploymentPhase` returns a
message alongside the reason, built from the failing workload's own status
("workload hello-world-collector failed: 0/1 nodes are available: 1
Insufficient memory…"), and `driveLifecycle` passes it to the store as
`EventMsg`. It reaches `deployment_events.message`; the log line dropped it.

Adding it as a `detail` field is the whole change. The message is never empty on
the failure path: the aggregator formats it for every failed workload, so the
field always carries at least the workload name.

`deployments.error_details` stays untouched. It is `json.RawMessage`, for
machine-readable context, and free text from a scheduler event does not belong
there.

## Migration

None.
