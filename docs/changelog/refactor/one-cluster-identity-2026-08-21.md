# One definition of cluster identity

## Summary

Three functions decided for themselves which values name the primary cluster, and they did not agree. `PlacementMismatch` counted the empty id and a `"default"` alias as the primary. `ClusterIDLabel` printed the word `default` for an unset id. Neither recognised the primary's own `clusters.id`, which is the form a deployment row carries once anything records it.

So a deployment recorded on the primary by id read as a move away from an unrecorded target, and the migrator would tear it down to redeploy it on the cluster it was already running on. `admingrpc` flagged the same pair as a placement mismatch.

## Design

**`internal/clusterid` owns the rule.** A routing target is a `clusters.id`, or the empty string for a target that was never recorded, which means the configured primary. Two targets name the same cluster when they resolve to the same id. No reserved word names a cluster: nothing writes `"default"`, so the alias is gone rather than carried.

**The primary is held, not passed.** The rule is a `Resolver` built once from configuration, so a placement decision that later needs another input grows the Resolver instead of every signature between the config and the comparison. The zero value carries no primary, which is the local mode with no cluster config, so an unwired caller degrades to today's behaviour rather than panicking.

`clusterplacement` and `admingrpc` compare through it, and the `Migrator` holds one. The package has no dependencies so every layer can import it.

**The binding table lands unused.** An account will hold a set of allowed clusters rather than a single pinned one, in `account_clusters`: one row per cluster, with a partial unique index so the database, not application code, guarantees at most one default per account. Nothing reads or writes it yet. It ships ahead of the code so the schema change is reviewed and applied on its own, leaving the change that starts using it to carry only the drop of the column it replaces.

## Migration

Nothing required. `account_clusters` is created empty and no code path touches it, so an account keeps deploying exactly as it does today. Two behaviour changes, both corrections:

- A deployment whose row carries the primary's id no longer reads as needing migration away from an unrecorded target, so the migrator stops tearing one down to rebuild it in place.
- Admin messages and a patched spec name the primary by its id instead of the word `default`.
