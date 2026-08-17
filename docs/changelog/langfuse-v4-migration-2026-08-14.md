# Langfuse v3 → v4 migration

## Summary

Langfuse ships a v4 major version that replaces its ClickHouse `traces`/`observations` tables with a unified `events_full` table and deprecates the v1 read/ingestion APIs. Astro's Langfuse integration reaches past the stable public API in one place — `apps/astro-server/internal/langfuse/client.go` calls legacy read endpoints (`/api/public/traces`, `/api/public/observations/{id}`, `/api/public/metrics`, `/api/public/metrics/daily`) — so the upgrade needs a staged rollout rather than a version bump.

`docs/01-spec/insights-rollup-spec.md`'s producer does not query ClickHouse directly, despite an earlier draft of the migration plan claiming otherwise. It calls `client.GetMetrics()` like every other caller, so it inherits `client.go`'s v4 support rather than needing its own rewrite.

## Design

The migration plan (`docs/06-plan/langfuse-v4-migration.md`) stages the rollout through Langfuse's `LANGFUSE_MIGRATION_V4_WRITE_MODE` env var: `legacy` (no schema change) → `dual` (writes to both v3 and v4 tables) → `events_only` (v4 tables only, old ones stop being written). Dev and dev-tenant are applied and running in `dual` mode; preview and prod are prepared at `legacy` mode but not yet applied.

`client.go` selects between Langfuse's legacy REST API and its v4 API (`/v2/observations`, `/v2/metrics`) via a `traceReader` interface (`v3Reader` / `v4Reader`), chosen once in `NewClient` from `LANGFUSE_USE_V4_API` rather than checked per call. Both implementations share the same HTTP transport; retiring v3 later means deleting `v3Reader` and its branch in the constructor, not hunting down scattered checks.

v4 is the default read path. `LANGFUSE_USE_V4_API` is an opt-out, not an opt-in: only an explicit false value (`false`, `0`, `FALSE`) selects `v3Reader`, and an unset or unparseable value keeps v4. The direction is deliberate. A cut-over environment reads v4 without needing to carry config, and a typo in the variable degrades to the intended path rather than silently reverting that environment to v3. The cost of that direction is that an environment whose Langfuse still writes in `legacy` mode has to opt out by hand.

Each reader's wire contract is covered by its own tests. Both are reachable regardless of `LANGFUSE_USE_V4_API` in the ambient environment, because the test helpers pin the reader under test instead of building it through `NewClient`.

Several v4 API behaviors don't map 1:1 onto v3 and needed correcting for, found by testing live against a real v4 instance rather than assumed from documentation:
- `tags=`, `fromTimestamp=`/`toTimestamp=`, and `orderBy=` are silently accepted but ignored by `/v2/observations` — filtering must go through a JSON `filter=` array instead (`fromStartTime`/`toStartTime` for time range), and sort order is applied client-side.
- `fields=` on `/v2/observations` is additive (`fields=basic,io` returns both core fields and input/output together), unlike the mutually-exclusive-looking behavior an initial pass concluded from testing single field groups in isolation.
- Pagination is opaque-cursor-based (`meta.cursor`), not v3's page numbers.
- `/v2/metrics` has no `view:"traces"` equivalent. Reproducing trace-level counts requires `view:"observations"` filtered to `isRootObservation=true` — but that filter must *not* apply to cost/token measures, since those live on child `GENERATION` spans and don't roll up onto the root. A query mixing both measure families can't be satisfied by one filter choice.
- Grouping `/v2/metrics` by a high-cardinality dimension like `userId` requires `orderBy` and `config.row_limit`, and that limit is capped at 1000 server-side — a real product-level constraint for the Insights rollup's per-user grouping on large accounts, left as an open decision rather than silently patched.

`apps/astro-server/internal/langfuse/provisioner.go`, which writes directly to Langfuse's Postgres to provision projects/keys, needs no changes — the `organizations`/`projects`/`api_keys` schema is unchanged between v3.221.1 and v4.11.0.

## Migration

v4 is now the default read path, and every environment's Langfuse writes in `dual` mode, so no per-environment opt-out is required. `astro-infra` moves preview and prod from `legacy` to `dual` in `helm/values/common/langfuse.yaml.tpl`; dev and dev-tenant were already there. Apply that infra change before or with this one. An environment left in `legacy` mode reads v4 ClickHouse tables that nothing populated, which surfaces as empty traces and empty metrics rather than an error, because the v4 endpoints answer a miss with `200` and an empty list. The escape hatch for that case is `LANGFUSE_USE_V4_API=false` on `astro-server`.

**Traces predating the `dual` rollout are not readable in preview or prod.** Langfuse's historic backfill stays disabled, so `dual` mode writes new events only, and older events remain in the v3 tables that the v4 API does not read. Either run the backfill, or set `LANGFUSE_USE_V4_API=false` until it finishes. See `astro-infra`'s `docs/decisions/0009-langfuse-v4-dual-write-preview-prod.md`.

Rollback needs no infra change: `dual` mode keeps writing the v3 tables, so setting `LANGFUSE_USE_V4_API=false` restores the v3 read path. That stops being true at `events_only`. See `docs/06-plan/langfuse-v4-migration.md` for the remaining open risks (Insights rollup's `userId` grouping ceiling, unverified cross-span cost attribution under v4) before any environment moves to `events_only`.
