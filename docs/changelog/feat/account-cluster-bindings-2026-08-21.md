# The account_clusters store

## Summary

`account_clusters` holds the set of clusters an account may deploy to. This is the code that reads and writes it. Nothing calls it yet, so the invariants it keeps can be reviewed and tested against a real database before any placement decision depends on them.

## Design

**Exactly one default per bound account.** The partial unique index already guarantees at most one. The other half, at least one once the account has any binding, depends on how many rows survive a write, which no index can express, so `ClusterBindings` holds it inside the same transaction: `Add` gives the first binding the flag whether the caller asks or not and never clears it off the current default, and `Remove` promotes another binding when it deletes the default one. Moving the flag is `SetDefault`'s job, and it refuses a cluster the account is not bound to.

`DefaultClusterID` is therefore total for a bound account. Its fallback covers rows edited outside those two, and picks the binding `Remove` would promote rather than whichever the caller collected first, so placement never depends on slice order.

**Reading an account writes the primary in.** An unbound account would otherwise be a third state to reason about: not bound, but treated as bound to the primary. `List` writes that binding the first time it finds none, so the table is the whole answer. `Add` does the same before inserting, so an account's set does not depend on whether anything read it first. Nothing to bind is still valid: with no `clusters` row for the primary, the insert matches nothing and the account stays unbound.

**Detaching a busy cluster is refused.** `Remove` counts the account's active, failed, and pending deployments on the cluster first and returns `ErrClusterInUse` if any remain.

**The invariants are tested against Postgres.** A partial unique index, an `ON CONFLICT`, and a conditional promotion are not things a mocked driver can check: it replays whatever rows a test hands it and would accept all three even if the statements were wrong. Those tests live in the integration suite.

## Migration

Nothing required. No caller reaches this code, and the table it reads is empty.
