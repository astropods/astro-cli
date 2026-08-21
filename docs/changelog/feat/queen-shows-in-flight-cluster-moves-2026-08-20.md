# Show a deployment's in-flight cluster move

## Summary

A deployment moving between clusters routes to its old cluster until the migration tears that down and deploys. During that window the admin console shows the source cluster with no sign that a move is under way, so an operator reading the row sees a stale answer with nothing marking it as stale.

## Design

**The pair already says it.** A move in flight is a deployment that is pending, whose stored spec names a different cluster than the row it routes to. `InFlightMove` names that pair and returns the destination. Nothing new is stored: a column would be a second copy of the same fact to keep in step, and it would widen a scan that ten test files enumerate.

Comparison goes through `clusterid.Resolver`, so a spec written before placement was recorded compares equal to the primary rather than reading as a move away from it. An unparseable spec reads as settled, because a display field should not decide that something is moving on the strength of JSON it could not parse.

**Derived on the way out.** `AdminDeployment.migrating_to_cluster_id` is computed when the admin API builds the row. The console shows the destination beside the cluster the deployment still routes to, as a badge in the list and a clause on the detail page.

**Nothing produces this state yet.** The migrator writes the row and the spec in one statement, so the two never disagree while a migration runs, and the field stays empty. It lands ahead of the deploy path that will leave the row on the source cluster for the migrate job, so the proto field and the console reach production before the server starts producing moves for them to describe.

## Migration

None. The proto field is additive, and an empty value renders nothing.
