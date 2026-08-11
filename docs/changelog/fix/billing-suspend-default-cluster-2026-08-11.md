# Suspend deployments that live on the primary cluster

## Summary

Billing suspension could not stop almost any deployment. `BillingSuspendWorker`
resolved the target cluster with `Registry.Get(dep.EffectiveClusterID())`, and
`Registry.Get` rejects an empty id. A deployment row with no `cluster_id` lives
on the primary cluster, which is 23 of the 24 active rows in preview, so the
worker logged a resolution error per deployment and finished reporting
`suspended=0`.

The first suspension this system ever ran hit it exactly:

```
ERROR billing suspend: cluster client  deployment_id=i17-w06-bu3
      error="registry.Get: empty cluster id"
INFO  billing suspend  suspended=0 total=1
```

An account with no credit and no card therefore kept running, while its status
row and the customer-facing banner both said its agents were stopped.

## Design

Resolution moves into `suspendClusterClient`, which falls back to
`Registry.Default()` for a row with no `cluster_id` and errors when no primary
is configured rather than resolving to a nil client.

This is the convention every other resolver already follows:
`handlers.clusterClientForDeployment`, `deployer.clusterClientForKey`,
`deploycontroller.clusterClient`, and `admingrpc.deploymentClusterClient` all
special-case the empty id. The suspend worker was the only caller that did not,
which is why the gap survived: the same deployments suspension could not touch
were deployed, watched, and undeployed without trouble.

That makes five copies of the same three lines. Consolidating onto a
`Registry.GetOrDefault` would be reasonable, but it would touch four working
call sites to fix one broken one, so it is left as a follow-up.

`TestSuspendClusterClient_DefaultsWhenRowHasNoCluster` fails against the
previous resolver with the production error verbatim, `registry.Get: empty
cluster id`. A second case covers a missing primary.

## Migration

None. No configuration or data changes; rows keep a null `cluster_id`.
