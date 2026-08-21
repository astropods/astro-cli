# Authorize image pulls against an account's bindings

## Summary

The registry decides whether a cluster may pull an account's image by checking that the account is homed there. Homing read `accounts.cluster_id`, which holds exactly one cluster per account. Bindings no longer live there: `account_clusters` holds them, and it holds a set.

An account allowed on two clusters would be homed on at most one. Pods on the other cluster would pull the same image and be refused, so they fail with ImagePullBackOff after a deploy the placement rule accepted.

## Design

**A bound account's set is exhaustive.** A cluster the account is bound to may pull; one it is not, may not. The primary is no exception: it is an ordinary row in `account_clusters` under cluster-config boot sync, so confining an account to one region also stops the primary from pulling its images. Exempting it would leave the pull rule wider than the placement rule for the one cluster most accounts use.

**An account with no bindings is homed on the primary, not everywhere.** `account.IsAllowed` reads an empty set as unrestricted, which is safe on the deploy path because every caller reaches it through the read that materializes the primary first. There, empty means no cluster is registered at all. The registry has no such guarantee: it cannot write, so empty is the ordinary state of an account nothing has read yet. Carrying the unrestricted reading across would let any authenticated cluster pull for every such account.

So the registry computes what materializing would record rather than what an unmaterialized read returns: bindings when the account has them, the primary when it has none. An additional cluster is refused, which is sound because running an account there takes an operator action that writes the bindings first.

The check rides along as two predicates on the account lookup the resolver already makes, so it stays one round trip. The namespace is still an account id or an account name depending on its shape, and either way the lookup hits a unique index.

**The sentinel resolves to the real id.** A CPC issued before boot sync carries the reserved `primary` identifier for the cluster that `account_clusters` records under its configured id. Homing resolves one to the other, so a bound account is not read as bound to some other cluster.

**An unregistered primary ignores bindings.** With no cluster-config, the primary has no `clusters` row and so cannot appear in `account_clusters` at all. An operator has no way to express the binding an exhaustive check would demand, so that case authorizes any account that exists.

## Migration

None. Every account either has no bindings, and is homed on the primary exactly as before, or was bound by an operator action that recorded where it runs.
