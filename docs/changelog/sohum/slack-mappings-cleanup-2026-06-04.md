# Slack observed users — PR 3: cleanup (destructive)

## Summary

PR 3 of the [slack mappings split proposal](../../proposals/slack-mappings-table-split.md). Stacks on [#1283](https://github.com/astropods/astro/pull/1283).

The destructive cleanup. Deletes the orphaned observed rows out of `slack_identity_mappings`, removes the CHECK constraints + `source` discriminator + nullable `workos_user_id` that PR 1 introduced as transitional scaffolding, and retires the one-shot port worker. After this PR, `slack_identity_mappings` is a pure oauth-link table and `slack_observed_users` is the sole home of the observed-anonymous directory.

## Design

### One-shot cleanup worker

`SlackMappingsCleanupWorker` (River kind `slack.mappings_cleanup`) runs a single SQL:

```sql
DELETE FROM slack_identity_mappings WHERE workos_user_id IS NULL;
```

Filter is `workos_user_id IS NULL` rather than `source = 'observed'` — same effect today (the CHECK ties the two together), but this SQL survives the follow-up schema migration that drops the `source` column. Once `workos_user_id NOT NULL` is restored, the DELETE matches zero rows on every subsequent invocation; idempotent.

Marker-gated by `slack_mappings_cleanup_marker`. Three-layer "never runs twice" guarantee mirrors the directory backfill: `main.go` enqueue gate, worker entry re-check, `UniqueOpts` on enqueue.

### Schema diff

`slack_identity_mappings` loses three things in the declarative schema:

- `slack_identity_mappings_source_check` CHECK constraint
- `slack_identity_mappings_workos_required_for_oauth` CHECK constraint
- `source` column

And gains one:

- `workos_user_id` returns to `NOT NULL`

Plus `slack_observed_port_marker` table is dropped (PR 1's port worker is gone in this PR), replaced by `slack_mappings_cleanup_marker`.

### Code cleanup

- `SourceOAuth` and `SourceObserved` constants — deleted.
- `Mapping.Source` field — deleted.
- `Upsert` SQL — dropped `source` column from INSERT and ON CONFLICT SET clause.
- `ListByWorkOSUser` / `ListByWorkOSUsers` — dropped `source` from SELECT and Scan.
- `Lookup` — dropped redundant `workos_user_id IS NOT NULL` filter (after the schema migration, the column is `NOT NULL` so the filter never excluded anything).
- `ListLinkedAccountTeams` — dropped redundant `sim.workos_user_id IS NOT NULL` filter for the same reason.
- `IsObservedPortComplete` / `MarkObservedPortComplete` / `PortObservedRowsToNewTable` — deleted along with the port worker.
- New: `IsMappingsCleanupComplete`, `MarkMappingsCleanupComplete`, `DeleteOrphanedObservedRows`.
- `SlackObservedPortWorker` + tests — deleted.
- New: `SlackMappingsCleanupWorker` + tests.
- `enqueueSlackObservedPortIfNeeded` (main.go) → `enqueueSlackMappingsCleanupIfNeeded`.

## Migration

**Operator sequence — strict ordering matters.**

1. Merge PR 3.
2. **Deploy the app first** (code change). The cleanup worker is now in the binary; the schema migration has NOT run yet so `source` column + CHECK constraints + nullable `workos_user_id` still exist in prod.
3. **Pod restart** triggers `enqueueSlackMappingsCleanupIfNeeded`. The worker runs the DELETE, writes the marker.
4. **Verify** the cleanup completed:
   ```sql
   SELECT completed_at FROM slack_mappings_cleanup_marker;
   SELECT COUNT(*) FROM slack_identity_mappings WHERE workos_user_id IS NULL;
   -- Expect: one marker row, zero NULL rows.
   ```
5. **Trigger SQL Migrate (Prod)** workflow. Atlas diffs schema.sql against prod and applies: drop CHECK, drop `source` column, `ALTER COLUMN workos_user_id SET NOT NULL`, replace port-marker table with cleanup-marker table.

If step 5 runs before step 3, Atlas's `ALTER COLUMN SET NOT NULL` will fail because rows with `NULL workos_user_id` still exist. Safe-fail.

### Why the schema migration can't happen at deploy time

Atlas Migrate is a separate manually-triggered CI workflow (`sql-migrate.yml`), not part of the app deploy pipeline. That separation is what gives ops the ability to sequence step 3 before step 5. The changelog above captures the manual gate.

## Why this is safe to run right after PR 2

- PR 2's cutover stopped writing observed rows to `slack_identity_mappings`. The rows the cleanup deletes are a frozen snapshot — no new ones since PR 2 deployed.
- PR 2's UNION read path queries both tables. After the DELETE, the slack_identity_mappings branch returns only oauth rows; the observed-row resolution moves entirely to `slack_observed_users`. Same end result for the merge logic.
- The schema migration is independent of read/write paths in this PR — code already operates without referencing the `source` column or relying on `workos_user_id IS NULL` rows.
