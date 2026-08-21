# Authorize image pulls against an account's bindings

## Summary

The registry decides whether a cluster may pull an account's image by checking that the account is homed there. Homing read `accounts.cluster_id`, which holds exactly one cluster per account. Bindings no longer live there: `account_clusters` holds them, and it holds a set.

An account allowed on two clusters would be homed on at most one. Pods on the other cluster would pull the same image and be refused, so they fail with ImagePullBackOff after a deploy the placement rule accepted.

## Design

**One rule, not two.** Homing asks the question `account.IsAllowed` asks on the deploy path, in SQL: an account with no bindings is unrestricted, and once it has any the set is exhaustive. A cluster that may run an account's workload may fetch its image, and one that may not, may not.

The primary is no exception. It is an ordinary row in `account_clusters` under cluster-config boot sync, so confining an account to one region also stops the primary from pulling its images. Exempting it would leave the pull rule wider than the placement rule for the one cluster most accounts use.

The check rides along as a predicate on the account lookup the resolver already makes, so it stays one round trip. The namespace is still an account id or an account name depending on its shape, and either way the lookup hits a unique index.

**The sentinel resolves to the real id.** A CPC issued before boot sync carries the reserved `primary` identifier for the cluster that `account_clusters` records under its configured id. Homing resolves one to the other, so a bound account is not read as bound to some other cluster.

**An unregistered primary skips the check.** With no cluster-config, the primary has no `clusters` row and so cannot appear in `account_clusters` at all. An operator has no way to express the binding an exhaustive check would demand, so that case resolves the account and stops there.

## Migration

None. An account with no bindings is unrestricted, so nothing changes until an operator binds a cluster, and binding materializes the primary alongside it.
