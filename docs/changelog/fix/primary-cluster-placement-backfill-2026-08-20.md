# Record the primary cluster on deployments that route there

## Summary

`deployments.cluster_id` is NULL for every deployment on the primary cluster, and holds a real id for every deployment anywhere else. A routing target therefore reads two ways, and code that compares one against a configured cluster id has to resolve the NULL first.

`RefreshClusterPullSecrets` does not. It resolves the request to the primary's literal id, then asks for the namespaces running on that id. The lookup treats an empty argument as "the NULL rows" and a non-empty one as an equality match, so the primary's own id matches nothing. Rotating the primary cluster's pull credential re-pushes the Secret to zero namespaces.

## Design

**One representation, backfilled once.** A pass records the configured primary's id on every non-undeployed deployment that has none. It runs on the deployment controller's leader, because one writer is enough and this should not repeat per API pod or per replica. It is idempotent: the NULL filter empties out and later passes update nothing. It also no-ops when no primary is configured, and when the configured primary has no `clusters` row, which is the local mode with no cluster config.

Undeployed rows keep their cleared id. Nothing runs there to place, and clearing on undeploy is deliberate.

**The namespace lookup stops branching.** With placement recorded, the query is one equality match. The empty-argument case is gone rather than reinterpreted, since the only caller resolves its target first and an unresolved target is not a cluster.

New deploys still record NULL for the primary until the deploy path resolves placement itself, so the pull-secret refresh covers what exists at the leader's pass and not rows created after it. Closing that gap belongs with the placement change.

## Migration

None. The backfill runs on startup and changes representation, not placement. No deployment moves and no workload restarts.
