# Multi-cluster accounts

## Summary

An account could be bound to exactly one cluster. `accounts.cluster_id` held a single value, and the deploy path overwrote every request's `target.cluster_id` from it, so an account's workloads could not span two regions for residency, capacity, or a cluster move.

This change replaces the single binding with a set. An account now has zero or more allowed clusters, one of which may be the default. A caller can name a cluster from that set at template time, and the server checks the choice instead of discarding it. Naming a different cluster for an agent that already runs somewhere moves it, through the migrate job that tears the old region down first.

This implements [the spec](../../01-spec/multi-cluster-account-support-spec.md) end to end: schema, store, placement logic, admin RPCs, the astro-queen editor, and the CLI and web pickers.

## Design

**Bindings live in `account_clusters`.** One row per allowed cluster, with a partial unique index (`WHERE is_default`) so the database, not application code, guarantees at most one default per account. A bound account also always has at least one: `AddCluster` gives the first binding the flag whether the caller asks or not and never clears it off the current default, and `RemoveCluster` promotes another binding when it deletes the default one. That half depends on how many rows survive the write, which no index can express, so the store holds it inside the same transaction. `DefaultClusterID` is therefore total for a bound account, and its fallback picks the binding `RemoveCluster` would promote instead of whichever the caller collected first: placement must not depend on slice order. The old column is dropped in this change; every row is already NULL, so there is nothing to carry over.

**Reading an account binds the primary.** Leaving an unbound account's set empty meant three states to reason about, not two: bound, unbound, and unbound-but-treated-as-if-bound-to-the-primary. Every consumer then had to know that the primary is allowed even when it is absent from the list, and they did not agree. The account read now writes the primary in as a real binding the first time it finds none, so the table is the whole answer.

`ListClusters` materializes it, and `AddCluster` does the same before inserting, so an account's set does not depend on whether anything happened to read it before an operator bound a second cluster. It is one write per account, and the read path is pure afterwards. Nothing to bind is still a valid state: with no `clusters` row for the primary, which is the local mode with no cluster config, the insert matches nothing and the account stays unbound.

**A bound account's set is exhaustive.** `IsAllowed` no longer waves the primary through. It is allowed because it is bound like every other cluster, so what a picker offers is exactly what a deploy accepts, and an operator who confines an account to one region gets a constraint that holds. Confining an account is two deliberate steps: bind the region's cluster, then remove the primary binding, which `RemoveCluster` refuses while anything still runs there. An account with no bindings at all is unrestricted rather than restricted to nothing.

**Placement splits in two, around the signature.** The deploy endpoint signs `response.template` verbatim, so the cluster choice has to be final before signing:

- `resolveTemplateClusterID` runs at template time. It honors `request.cluster_id` when the cluster is bound, falls back to the account's default when the caller names none, and rejects an unbound pick with 403 before anything is signed. The resolved cluster always comes from the account's own set, so the deploy-time check never contradicts what was just signed.
- `enforceAccountClusterPlacement` runs at deploy time, after signature verification. It only re-checks, never rewrites, closing the window where an operator unbinds a cluster between signing and deploy.

A refused pick names what the account can use instead, because the usual cause is a typo and "not allowed for this account" reads as a permissions problem an operator has to fix. Whether the id is unknown or merely unbound is deliberately not distinguished: the account's set is the whole list it may name, and reporting that some other cluster exists answers a question a tenant did not get to ask.

The two sides have to agree on what the allowed set contains, or a deploy fails on a cluster the template just handed out. That agreement now has one home.

**One definition of cluster identity.** Five functions each decided for themselves which values name the primary cluster, and no two agreed: the placement check accepted the empty id and the configured id, the migration comparison accepted the empty id and a `"default"` alias, the orphan predicate accepted the empty id and the configured id but not the alias. So `--cluster default` came back as "not allowed for this account", and a row holding `default` read as orphaned in the admin console while the migrator called it the primary.

`internal/clusterid` now owns the rule, with no dependencies so every layer can import it. A routing target is a `clusters.id`, or empty for a target that was never recorded; empty resolves to the configured primary, so a row written before placement was recorded compares equal to one written after. No reserved word names a cluster: `default` is an ordinary id, and a deployment on a cluster called that is treated like any other.

`orphanedDeploymentPredicate` restates `IsAllowed` in SQL because it runs in the database. That is the only copy, and it is marked as one.

Once placement lives in the database and every live deployment records an id, resolving an unrecorded target stops being something the placement path does. `IsAllowed` takes no primary at all: bindings are the whole answer, and a caller resolves its target before asking. The orphan predicate takes no argument either, so the admin console's count and list read only `deployments` and `account_clusters`. A row the backfill has not reached records no cluster, and both leave it alone rather than guess which one it meant, so nothing is flagged during the gap.

Resolution survives in three places, because an unrecorded target can still arrive there: an admin RPC argument, a spec being written, and a migration deciding whether it is a no-op. The last one is not cosmetic. Reading an unrecorded target as a different cluster there would tear a running workload down to rebuild it where it already was.

**The primary is held, not passed.** Threading a cluster id through every signature that might compare two targets made a one-line rule change cost twenty signatures, their call sites, and their tests, in both directions: adding the parameter and removing it were the same size of diff. `clusterid.Resolver` is now constructed once from configuration and held by whatever needs it, and `account.ClusterBindings` holds it alongside the `account_clusters` table, so no store method carries a cluster id it does not otherwise need. That took the parameter from twenty-one places to three, none of them placement: the backfill statement, the cluster-config sync, and the K8s registry's own field. A placement rule that later needs another input grows the Resolver, and no caller changes.

The zero `Resolver` carries no primary, which is exactly the local mode with no cluster config. A forgotten wire degrades to that rather than panicking.

Which clusters an account may be bound to carries no constraint yet: `AddAccountCluster` checks only that the cluster exists. Nothing stops an operator from binding a cluster in the wrong region, so that stays an operator judgment. A residency or region constraint is a follow-up design, not something this change guesses at.

**Detaching a busy cluster is refused.** `RemoveCluster` counts the account's active, failed, and pending deployments on the cluster first and returns `ErrClusterInUse` if any remain, matching how cluster deregistration already refuses while deployments reference a row. Migrating those deployments stays a separate, explicit step.

**Moving a live deployment goes through the migrate job.** A deploy that names a different cluster than the one its deployment runs on cannot simply deploy there: `SaveDeployment` overwrites `deployments.cluster_id` and the deploy worker only ever applies to the cluster the row names, so the workloads on the old cluster would keep running with nothing to tear them down. Such a deploy enqueues `deployment.migrate_cluster` instead of `deployment.deploy`. The migrate job tears down the source cluster, patches the spec, updates routing, and enqueues the deploy itself, so a second deploy job would race it.

**A move in flight is readable, and owns the routing.** The row and the spec already say one is under way: the deploy records the target in the spec and leaves `cluster_id` on the source for the migrate job. `inFlightMove` names that pair rather than adding a column, which would be a second copy of the same fact to keep in step, and would widen a scan ten test files enumerate. A deploy that names a third cluster while a move is under way is refused with 409 instead of racing a second migration whose loser's target is silently dropped. Re-submitting the same move is not a conflict, so a retry still works.

A superseded migration repoints the spec along with the row, so it stops naming a target nothing is moving to and the pair reads as settled again.

`AdminDeployment.migrating_to_cluster_id` carries the same reading to astro-queen, which shows the destination beside the cluster a deployment still routes to. It is derived on the way out rather than stored, so there is nothing to fall out of step with the spec and the row.

**A migration reports what it did.** The deploy that asks for one has already set the deployment pending and answered its caller, so a pending row is waiting on a job that only that attempt will enqueue. `MigrateDeployment` returned a single boolean for every no-op, which hid two cases that left it waiting: something else rerouted the row first, and the status guard lost its race after the source cluster was already torn down. It now returns a named outcome and whether a job exists. A superseded migration sends a pending deployment to where the row now points, since nothing was torn down yet. Losing the status race is an error rather than a silent success, so the job retries instead of reporting done over a workload that is down.

The row keeps its old `cluster_id` until that job runs, because `MigrateDeployment` reads the source cluster off the row and would otherwise see no change and skip. Everything else about the deploy commits normally: only the routing waits. A redeploy that names no cluster stays where it is rather than drifting to the account default, so it never migrates by accident.

**The form says when a deploy is a move.** The helper text under the region select changes when the selection differs from where the agent currently runs: deploying will move it, restart it, and tear down its current region. The comparison uses the cluster the picker resolved rather than the raw id, so an agent on the primary (recorded as an empty id) does not read as moving.

**Only query when it matters.** The deploy path skips the binding lookup entirely when the submitted target is empty, which after materializing only happens where no cluster is registered at all. That keeps the common single-cluster deploy at its current query count.

**Image-pull authorization follows placement.** The registry decides whether a cluster may pull an account's image by checking that the account is homed there, which read `accounts.cluster_id`: exactly one cluster per account. An account bound to two clusters would have been homed on one, so pods on the other would fail with ImagePullBackOff. Homing now reads `account_clusters`, so the pull rule and the placement rule are the same rule. The registry keeps its own primary sentinel for the mode where the primary has no `clusters` row and so cannot be bound.

**Cluster metadata rides on the account read.** `GET /accounts/:account` gains `allowed_clusters` (cluster id, region, default flag), joined from `account_clusters`. A picker needs a list with display data; the template endpoint's job is to resolve one choice. The field is omitted only where no cluster is registered, and it inherits that endpoint's existing public visibility: a cluster's region is no more sensitive than the display name and location already served there.

**Admin manages a set, not a value.** `SetAccountCluster` splits into `AddAccountCluster`, `RemoveAccountCluster`, `SetAccountDefaultCluster`, and `ListAccountClusters`, all returning the resulting list so the caller never re-reads.

**Bulk migration is gone.** `MigrateAccountDeployments` moved every deployment an account owned onto one cluster in a single operator action. With a region per deployment that destination is no longer well defined, and sweeping someone's workloads between regions is not an operator's call to make. `RemoveAccountCluster` still refuses while deployments remain, and now says to move or undeploy them rather than naming an RPC that no longer exists.

**Mismatch becomes orphaned.** A deployment used to be flagged when its cluster differed from the account's single binding. It is now flagged when its cluster is absent from the account's allowed set, which is the same check generalized: a one-item set behaves exactly as before. The predicate moves into SQL (`orphanedDeploymentPredicateSQL`) so the count and the list share one definition. `AdminDeployment.account_cluster_id` becomes `account_cluster_ids`, and `placement_mismatch` becomes `placement_orphaned`.

**Re-apply relocates an orphan.** An orphaned deployment cannot be re-applied where it sits, so `ReapplyDeployment` migrates it to the account's default cluster. A deployment on an allowed cluster re-applies in place, unchanged.

**The picker only appears when there is a choice.** astro-cli prompts for a region when the account has two or more allowed clusters and the shell is interactive, takes `--cluster` otherwise, and falls back to the account default when the lookup fails or the shell is not a TTY, so CI never blocks on a form. astro-client shows a region select fed by the account read it already makes. Neither offers a choice to a single-cluster account: the CLI prompts for nothing, and the client shows the one region without controls.

## Migration

No data migration and no operator action. `account_clusters` starts empty and fills itself: each account picks up its primary binding the first time something reads it, and keeps deploying to the primary exactly as it does today. `TemplateRequest.cluster_id` and `AccountResponse.allowed_clusters` are additive, so older CLI and client versions keep working.

`accounts.cluster_id` is dropped, and it needs no backfill either: every row is already NULL, so there are no bindings to carry over.

**`DEFAULT_CLUSTER_ID` no longer moves accounts.** The database now answers which clusters an account may use, and the variable is only read to seed a binding for an account that has none, and to say which cluster an unrecorded routing target meant. Changing it used to re-home every unbound account implicitly, because the deploy path consulted it on every request. It no longer does: an account keeps the binding it was given, and a new value only reaches accounts nothing has read yet. Moving an account is now `AddAccountCluster`, `RemoveAccountCluster`, and a migration per deployment. Renaming the primary cluster still needs the variable and the cluster config to change together, which boot sync already refuses to do by halves.

`deployments.cluster_id` is backfilled to the primary's id where it was NULL, so a routing target no longer has to be resolved against configuration to be compared. It changes representation, not placement. The pass runs on the deployment controller's leader, since one writer is enough and this should not run once per replica or per API pod, and it is idempotent: the NULL filter empties out and later passes do nothing. Undeployed rows keep their cleared id, which is deliberate — nothing runs there to place.

The temporary EU badge in astro-client (a rollout confirmation aid) was the last reader of that column and is deleted here, along with the `cluster_id` plumbing through the profile and auth payloads.

The region is now editable on Configure, and nothing about an existing deploy changes unless one names a different cluster than the one its deployment runs on.

Breaking for astro-queen only, which deploys in lockstep with the server: the renamed `AdminDeployment` fields and the new required `target_cluster_id`.
