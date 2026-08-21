# Managing an account's clusters from astro-queen

## Summary

An account's cluster was a single pinned value an operator set with `SetAccountCluster`. Now that an account holds a set of allowed clusters, the admin surface manages a set instead, and the deployment view flags a deployment against that set rather than against one value.

Nothing a tenant touches changes. The deploy path still ignores the set; wiring it up comes next.

## Design

**Admin manages a set, not a value.** `SetAccountCluster` splits into `AddAccountCluster`, `RemoveAccountCluster`, `SetAccountDefaultCluster`, and `ListAccountClusters`, each returning the resulting list so the caller never re-reads. The astro-queen account page edits that list.

**Mismatch becomes orphaned.** A deployment used to be flagged when its cluster differed from the account's single binding. It is flagged now when its cluster is absent from the account's allowed set, which is the same check generalised: a one-item set behaves exactly as before. `AdminDeployment.account_cluster_id` becomes `account_cluster_ids`, and `placement_mismatch` becomes `placement_orphaned`.

**Bulk migration is gone.** `MigrateAccountDeployments` moved every deployment an account owned onto one cluster in a single operator action. With a set there is no single destination, and sweeping someone's workloads between regions is not one click's worth of decision. `RemoveAccountCluster` refuses while deployments remain and says to move or undeploy them.

**The RPCs take an interface, not the store.** The account RPCs depend on a `clusterBindingStore`, so their tests state what the table was asked to do rather than which statements ran, and a store that grows a query cannot break a test about an admin action.

## Migration

Breaking for astro-queen only, which deploys in lockstep with the server: the renamed `AdminDeployment` fields, and `SetAccountCluster` and `MigrateAccountDeployments` replaced by the four RPCs above.
