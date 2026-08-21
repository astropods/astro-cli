# Read image-pull homing straight off the bindings

## Summary

Homing carried a fallback for an account with no bindings: route it to the primary, and refuse every other cluster. That covered a gap the registry could not close itself. It reads `account_clusters` and cannot write it, and nothing guaranteed a row existed, so an unbound account was the ordinary state of one nothing had read yet.

Account creation and the startup backfill now establish the set, so the fallback describes a state that no longer occurs.

## Design

**Membership is the whole answer.** A cluster may pull an account's images when the account is bound to it. The query drops the second predicate that asked whether any binding exists, and the branch that interpreted it goes with it.

An unbound account is now refused everywhere rather than routed to the primary. That is the point: with the set established, absence means the account genuinely is not homed there, and reading it as "not read yet" would keep the primary open to accounts an operator has confined elsewhere.

**One exception survives.** With no cluster config the primary has no `clusters` row, so nothing can be bound to it and an exhaustive check would refuse every account. That case authorizes any account that exists. It is a distinct mode rather than a fallback: no writer can populate bindings there.

The `primary` sentinel still resolves to the configured cluster id before the check, so a credential issued before boot sync names the same cluster a bound account records.

## Migration

Deploy after the backfill has run. Until every account is bound, this refuses pulls for accounts the backfill has not reached, which surfaces as ImagePullBackOff. Nothing else changes: a bound account is authorized on exactly the clusters it was authorized on before.
