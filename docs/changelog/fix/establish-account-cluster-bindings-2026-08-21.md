# Give account_clusters a writer

## Summary

`account_clusters` answers which clusters an account may use, and nothing establishes it. The only writer that fills it for an ordinary account is the materialization inside the account read, which runs when something happens to read that account. Account creation writes `accounts`, `account_members`, and `account_profile`, and no bindings. There is no backfill.

So an empty binding set carries two meanings that cannot be told apart: no cluster is registered at all, or nobody has read this account yet. Each reader picks one. `IsAllowed` reads empty as unrestricted, the orphan predicate reads it as not orphaned, and the registry's image-pull check cannot afford either reading, because for it "unrestricted" means any authenticated cluster may pull that account's images.

## Design

**Creation binds the primary.** `Create` writes the binding in the transaction that writes the account, so the set exists before anything can read it. `CreateWithoutOwner` does the same for organizations synced from WorkOS. Both go through the guarded insert the read path already used, so an account created while no cluster is registered still binds nothing.

**A backfill covers the accounts that predate this.** One idempotent statement binds the primary to every account that has none, on the deployment controller's leader, next to the placement backfill. Its filter empties out, so later passes do nothing. It leaves a bound account alone, including one an operator has confined to another region.

**The read stops writing.** Listing an account's clusters was what materialized them, so a read issued an insert and an empty set meant "empty, unless something has read this account". With creation and the backfill establishing the set, the read is a plain select.

Adding a cluster still binds the primary first, inside its own transaction. That is a write establishing the invariant at a write site, not a reader repairing it: without it, binding a second cluster to an account that somehow has none would confine it to that one cluster.

**What that buys.** Empty now means one thing: no cluster is registered, which is the local mode with no cluster config. `IsAllowed` and the orphan predicate already read that case correctly, so they are unchanged and are now right for one reason instead of two. A reader that cannot write, such as the registry's image-pull check, no longer has to model materialization it cannot perform.

## Migration

None. The backfill runs on startup and records the binding each account already routes to, so no placement changes and no workload restarts.
