# Stop reporting a cluster nothing sets

## Summary

`accounts.cluster_id` has no writer. `SetAccountCluster` was the only one, and it now writes bindings to `account_clusters` instead. `AccountStore.SetClusterID` survived with no caller.

Two API payloads still report the column. The auth response and the profile response each carry a `cluster_id` per account, and astro-client turns it into an EU badge on the org settings header. The column is NULL for every account, so the field is omitted from every response and the badge never renders. All three describe a placement decision that no longer lives there.

## Design

**The reads go, the column stays.** Dropping the column is a schema change that belongs with the placement work. What can go now is everything that reads it into a response: the field on the auth account payload, the field on the profile account payload, the `AccountWithRole` field they both fill, and its column in the accounts-for-user query. `SetClusterID` goes with them, along with the tests that were its only caller.

One read remains. The single-account lookup still scans the column, and the deploy path still consults it to place a deployment. That read is the placement rule itself, so it moves when the rule does.

**The EU badge goes.** It confirmed a rollout by reading the same field. astro-client no longer models a cluster on an account at all, which leaves the deploy form as the one place that talks about where an agent runs.

**Deregistration names the right blocker.** A cluster delete is refused while an account is bound to it, and `account_clusters_cluster_id_fkey` is the constraint that refuses it now. Only the old `accounts_cluster_id_fkey` was recognized, so a bound account produced the catch-all "still referenced by accounts or deployments" instead of the specific error. Both names map to it while the old column exists.

## Migration

None. `cluster_id` disappears from the accounts array in the auth and profile responses, where it was already omitted for every account. The column and its foreign key are untouched.
