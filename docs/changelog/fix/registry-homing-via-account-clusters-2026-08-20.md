# Authorize image pulls against an account's bindings

## Summary

The registry decides whether a cluster may pull an account's image by checking that the account is homed there. Homing read `accounts.cluster_id`, which holds exactly one cluster per account. Bindings no longer live there: `account_clusters` holds them, and it holds a set.

An account allowed on two clusters would be homed on at most one. Pods on the other cluster would pull the same image and be refused, so they fail with ImagePullBackOff after a deploy the placement rule accepted.

## Design

**Membership is the whole answer.** A cluster may pull an account's images when the account is bound to it. That is the question `account.IsAllowed` asks on the deploy path, so a cluster that may run an account's workload may fetch its image, and one that may not, may not.

The primary is no exception. It is an ordinary row in `account_clusters` under cluster-config boot sync, so confining an account to one region also stops the primary from pulling its images. Exempting it would leave the pull rule wider than the placement rule for the one cluster most accounts use.

The check rides along as a predicate on the account lookup the resolver already makes, so it stays one round trip. The namespace is still an account id or an account name depending on its shape, and either way the lookup hits a unique index.

**The sentinel resolves to the real id.** A credential issued before boot sync carries the reserved `primary` identifier for the cluster that `account_clusters` records under its configured id. Homing resolves one to the other, so a bound account is not read as bound to some other cluster.

**An unregistered primary skips the check.** With no cluster config the primary has no `clusters` row, so it cannot appear in `account_clusters` and an exhaustive check would refuse every account. That case authorizes any account that exists. It is a distinct mode rather than a fallback: no writer can populate bindings there.

**An unbound account is refused.** Account creation and the startup backfill establish the binding set, so absence means the account genuinely is not homed on the requesting cluster. Reading it as unrestricted instead would let any authenticated cluster pull for every account, and routing it to the primary would keep the primary open to accounts an operator has confined elsewhere.

## Migration

Deploy after the account binding backfill has run. Until every account is bound, this refuses pulls for accounts the backfill has not reached, which surfaces as ImagePullBackOff until it does. Nothing else changes: a bound account is authorized on exactly the clusters it was authorized on before.
