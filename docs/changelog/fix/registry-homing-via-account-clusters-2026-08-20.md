# Authorize image pulls against an account's bindings

## Summary

The registry decides whether a cluster may pull an account's image by checking that the account is homed there. Homing read `accounts.cluster_id`, which holds exactly one cluster per account. Bindings no longer live there: `account_clusters` holds them, and it holds a set.

An account allowed on two clusters would be homed on at most one. Pods on the other cluster would pull the same image and be refused, so they fail with ImagePullBackOff after a deploy the placement rule accepted.

## Design

**One rule, not two.** Homing now asks whether the account has a binding to the requesting cluster, which is what `IsAllowed` asks on the deploy path. The pull rule and the placement rule read the same table, so a cluster that may run an account's workload may fetch its image.

The check rides along as an `EXISTS` on the account lookup the resolver already makes, so it stays one round trip. The namespace is still an account id or an account name depending on its shape, and either way the lookup hits a unique index.

**The primary keeps its sentinel.** The primary cluster has no `clusters` row in the mode this sentinel exists for, so nothing can be bound to it and a binding check would refuse every account. It authorizes any account that exists, which is the answer the NULL column gave before.

This does widen the primary. An account bound only to another cluster used to be un-homed on the primary and is now homed there. No account is bound away from the primary, so nothing changes in practice, and confining an account to one region is a placement decision the deploy path enforces.

## Migration

None. Bindings materialize as accounts are read, and every account keeps the primary binding it picks up, so the pull path answers as it does today until an operator binds a second cluster.
