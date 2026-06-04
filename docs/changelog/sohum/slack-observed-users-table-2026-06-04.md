# Slack observed users — table split, PR 1: add + populate

## Summary

PR 1 of the [slack mappings split proposal](../../proposals/slack-mappings-table-split.md). Introduces `slack_observed_users` as a separate table for the observed-anonymous Slack directory, dual-writes new entries into it from the live-ingest path, ports legacy observed rows from `slack_identity_mappings`, and fixes a load-bearing bug in the historical Langfuse backfill that was causing it to silently 400 in production.

This PR does **not** change reads. Insights still consults `slack_identity_mappings` for the directory join. PR 2 switches reads to `slack_observed_users`.

## Design

### New schema

```sql
CREATE TABLE slack_observed_users (
  team_id        varchar     NOT NULL,
  slack_user_id  varchar     NOT NULL,
  first_seen_at  timestamptz NOT NULL DEFAULT now(),
  last_seen_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (team_id, slack_user_id)
);
```

Pure derived cache. No identity columns, no `revoked_at`, no `source` discriminator. Truncatable without consequence; live-ingest refills on every `/authorize` call.

A second table, `slack_observed_port_marker`, gates the one-shot port worker — same singleton-row pattern as `slack_directory_backfill_marker`.

### Dual-write

`UpsertObserved` now writes the (team_id, slack_user_id) pair into both tables in sequence:

1. **Legacy `slack_identity_mappings`** (existing logic, unchanged).
2. **New `slack_observed_users`** — `INSERT ... ON CONFLICT (team_id, slack_user_id) DO UPDATE SET last_seen_at = now()`.

A legacy-write failure rolls back the in-process dedupe entry so the next call retries both. A new-table failure does NOT roll back dedupe — the legacy write succeeded, which is the read path PR 1 still serves. Gaps caused by occasional new-table failures are picked up by the port worker (run once per env) before PR 2's read switch.

### One-shot port worker

`SlackObservedPortWorker` (River job kind `slack.observed_port`) copies every active observed row out of `slack_identity_mappings` into `slack_observed_users` in a single SQL statement:

```sql
INSERT INTO slack_observed_users (team_id, slack_user_id, first_seen_at, last_seen_at)
SELECT team_id, slack_user_id, created_at, updated_at
FROM slack_identity_mappings
WHERE source = 'observed' AND revoked_at IS NULL
ON CONFLICT DO NOTHING;
```

Three-layer "never runs twice" guarantee mirrors `SlackDirectoryBackfillWorker`:
- `main.go` checks the marker before enqueueing.
- The worker re-checks the marker on entry.
- `UniqueOpts{ByArgs: true}` on the enqueue collapses concurrent enqueues from rolling-restart replicas.

Idempotent (`ON CONFLICT DO NOTHING`), so a re-run after marker deletion produces zero net change for rows already ported.

### Langfuse backfill bug fix

`distinctBareSlackUserIDs` (used by `SlackDirectoryBackfillWorker`) was sending an empty `fromTimestamp`/`toTimestamp` on its Langfuse query. `/api/public/metrics` returns 400 on empty timestamps — confirmed in the existing `metricsTimeRange` helper comment in `observability_langfuse.go`. The worker swallowed the error per-account and wrote zero observed rows, then wrote the completion marker anyway.

Impact in prod: every observed row visible in `slack_identity_mappings` today came from the live-ingest `/authorize` path, not the historical backfill. Pre-deploy Slack users whose last activity predates PR #1228 have no observed row → no `slack_team_id` on Insights responses → no deep link.

Fix: explicit 5-year `FromTimestamp`/`ToTimestamp` window in `distinctBareSlackUserIDs`. Mirrors the handler's fallback in `metricsTimeRange`.

## Migration

Ops sequence after this PR deploys:

1. **Atlas applies schema changes** — `slack_observed_users` + `slack_observed_port_marker` created.
2. **Pod restart** — boot-time enqueue queues `slack.observed_port`. Worker runs, copies legacy observed rows into the new table, writes the port marker.
3. **`DELETE FROM slack_directory_backfill_marker`** — required to take advantage of the Langfuse timestamp fix. Without this, the historical backfill stays gated and pre-deploy Slack users continue to lack observed rows.
4. **Second pod restart** — boot-time enqueue picks up the now-unblocked `slack.directory_backfill`. With the fix, the per-account Langfuse query succeeds and historical bare Slack user IDs get observed rows in both tables (via dual-write).

After step 4: `slack_observed_users` is fully populated. No user-visible change yet — PR 2 wires the read path.
