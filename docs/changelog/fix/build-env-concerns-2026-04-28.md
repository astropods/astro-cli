# Fix: `deployment_build_env` dual-write and backfill correctness

## Summary

Four correctness issues in the `deployment_build_env` dual-write (#852) are addressed before the follow-up PR drops `deployment_variables`. All four were non-blocking suggestions from review; together they harden the migration window guarantees.

## Design

### 1. Empty-targets parity

Variables declared with no `Targets` were written to `deployment_variables` but produced zero rows in `deployment_build_env` (no roles to fan out to). Dropping `deployment_variables` would have silently lost them. Both writers now agree: variables with no targets are skipped in both tables. Since such variables can't be consumed by any container, this is a no-op for runtime behavior.

### 2. `RepairNormalizedSpec` preserves build_env rows

`RepairNormalizedSpec` calls `SaveNormalizedSpec` with `ds.Variables = nil` to avoid re-inserting stripped secret values. The original code did an unconditional `DELETE FROM deployment_build_env` at the top of the variables block, which wiped existing rows even when no variables were being written. The DELETE now only runs when `len(ds.Variables) > 0`, so Repair leaves the table intact.

### 3. Backfill INSERT is idempotent under concurrent writers

The backfill's EXISTS-then-INSERT is a TOCTOU race: a concurrent dual-write (new deploy landing while the backfill runs) could insert a row between the check and the insert, causing a unique-violation error and marking that deployment as failed. The INSERT now carries `ON CONFLICT (deployment_id, role, env_name) DO NOTHING`, making the backfill safe under concurrent writers without an error.

### 4. `Work()` surfaces per-deployment failures to River

`Work()` previously returned `nil` regardless of how many per-deployment failures were logged. River treats a nil return as success, marks the job done, and never retries — a transient DB blip would leave affected deployments without `build_env` rows for 24 h with no visible signal. `Work()` now returns a non-nil error when `failed > 0`, causing River to retry the job.

## Migration

No schema changes. Deployed automatically on next server start. No user action required.
