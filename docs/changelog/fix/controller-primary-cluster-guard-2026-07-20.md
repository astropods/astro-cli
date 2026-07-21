# Fix: deployment controller skipped every primary-cluster deployment

## Summary

The Phase 1 deployment controller logged `watching cluster` but wrote no rows to
`deployment_workload_status` for primary-cluster deployments. Its cross-cluster
ownership guard compared the registry's primary id (`"primary"`) against a
deployment's `EffectiveClusterID()`, which is `""` for primary-cluster
deployments (`cluster_id` NULL). `"" != "primary"` was always true, so every
primary-cluster sync skipped before writing.

## Design

Added `canonicalCluster`, which collapses the primary cluster's two spellings —
`""` (from `EffectiveClusterID`) and `PrimaryClusterID` (`"primary"`, from the
registry) — to one value, and compares both sides of the guard through it.
Additional-cluster ids pass through unchanged, so cross-cluster flapping
protection is preserved. Regression test locks the normalization.

## Migration

None.
