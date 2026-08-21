# Report what a cluster migration did

## Summary

`MigrateDeployment` returned one boolean for every no-op. Three unrelated outcomes shared it: the deployment is not migratable, it already routes to the target, and the status guard lost its race after the source cluster was torn down. The last one reports success over a workload that is down, so the job never retries. The deployment stays pending, waiting on a job that only that attempt would enqueue.

## Design

**A named outcome replaces the boolean.** `MigrateDeployment` returns a result carrying one of `applied`, `already_on_target`, `not_migratable`, or `source_moved`, plus whether a deploy job now exists. The worker logs both, so job history distinguishes a genuine no-op from a stall.

**Losing the status race is an error.** The guarded routing update reports that the deployment's status changed between the read and the write. Teardown has already run by then, so treating it as done leaves nothing running and nothing queued. The error now propagates and River retries the job.

**A superseded migration lands the row where it now points.** When the job's source cluster no longer matches the row, something else rerouted the deployment first. A pending row is still waiting on a deploy and nothing has been torn down, so the job repoints the spec at the row's current cluster and enqueues the deploy there. An active row is already settled and keeps its placement.

**The worker holds a configured resolver.** The migrator compares two cluster ids through `clusterid.Resolver`, which reads an unrecorded target as the primary cluster. The worker built one with no primary configured, so an unrecorded target compared as a different cluster than the primary. Nothing enqueues such a job today, because admin re-apply skips deployments with no cluster recorded. This closes the gap ahead of the deploy path enqueuing migrations, rather than fixing an observed failure.

`ListDeploymentsNeedingMigration` goes away with it. Bulk account migration was its only caller.

## Migration

None. The job kind and its payload are unchanged.
