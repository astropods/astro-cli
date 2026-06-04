# Slack observed users — PR 2: cutover (read switch + drop dual-write)

## Summary

PR 2 of the [slack mappings split proposal](../../proposals/slack-mappings-table-split.md). Stacks on top of [PR #1281](https://github.com/astropods/astro/pull/1281).

Switches the Insights directory read path from `slack_identity_mappings` to a UNION across both tables (oauth identity from the legacy table, team_id fallback from `slack_observed_users`), and removes the PR 1 dual-write so `UpsertObserved` now writes exclusively to `slack_observed_users`.

Reads go live in this PR. PR 3 (cleanup) will then evict the now-stale observed rows from `slack_identity_mappings` and drop the related CHECK constraint + `source` column.

## Design

### Write path: drop the legacy `slack_identity_mappings` dual-write

`UpsertObserved` previously wrote to both tables in sequence. PR 2 keeps only the `slack_observed_users` insert. The in-process dedupe map remains — same purpose (chatty workspaces shouldn't burn a DB write per message) — but the legacy table's revoke/revival logic is gone from this code path entirely.

```go
// PR 2 cutover: slack_observed_users is the sole write target.
INSERT INTO slack_observed_users (team_id, slack_user_id)
VALUES ($1, $2)
ON CONFLICT (team_id, slack_user_id) DO UPDATE
SET last_seen_at = now()
```

A DB error rolls back the dedupe entry so the next call retries — same contract as before, just simpler (one write instead of two).

### Read path: UNION across both tables

`DirectoryEntriesForSlackUsers` now unions two sources, with priority ordering so the linked-table entry always wins when both have data:

```sql
SELECT DISTINCT ON (slack_user_id) slack_user_id, team_id, workos_user_id
FROM (
    SELECT slack_user_id, team_id,
           COALESCE(CASE WHEN revoked_at IS NULL THEN workos_user_id END, '') AS workos_user_id,
           (revoked_at IS NULL) AS active_flag,
           created_at,
           1 AS source_priority
    FROM slack_identity_mappings
    WHERE slack_user_id = ANY($1)
    UNION ALL
    SELECT slack_user_id, team_id, '' AS workos_user_id,
           TRUE AS active_flag,
           last_seen_at AS created_at,
           2 AS source_priority
    FROM slack_observed_users
    WHERE slack_user_id = ANY($1)
) combined
ORDER BY slack_user_id, source_priority, active_flag DESC, created_at DESC
```

Per slack_user_id, precedence is:

1. **Active oauth row** in `slack_identity_mappings` — `workos_user_id` set, `team_id` from oauth row. Drives both the linked-merge in `mergeLinkedSlackRows` and the deep link.
2. **Revoked oauth row** in `slack_identity_mappings` — `workos_user_id` masked to empty (post-disconnect spend mustn't fold back into the unlinked account), `team_id` from oauth row. Preserves the deep link for previously-linked-then-disconnected users.
3. **`slack_observed_users` row** — `workos_user_id` empty, `team_id` from observed-directory. Drives the deep link for unlinked Slack senders (the common case).

### Cache invariants

The Insights users-summary cache holds JSON bytes keyed by (account, endpoint, params). Cache entries written under PR 1's read path are still valid: they consulted `slack_identity_mappings` for the same identity data PR 2's UNION resolves to (with the same precedence). No mass invalidation required for the read switch.

Refreshes / cache misses naturally pick up the new path.

## Migration

Schema is unchanged from PR 1. Code-only PR.

Order of operations the operator already executed in PR 1 (port worker + Langfuse fix backfill) holds; this PR doesn't add new ops steps. After deploy:

- Live-ingest writes only `slack_observed_users`.
- Reads union both tables. Existing oauth identities and existing observed rows continue to resolve correctly.
- Stale observed rows in `slack_identity_mappings` are now harmless — write path no longer touches them, read path's UNION still includes them but the `source_priority` ordering prefers either the (more authoritative) linked entry or the (more current) `slack_observed_users` entry. PR 3 deletes them.

## Why "stop dual-writing now" is safe

Two facts make the read-switch self-sufficient:

1. **PR 1's one-shot port worker ran**, copying every active observed row out of `slack_identity_mappings` into `slack_observed_users`. So every historically-observed slack_user_id has a row in the new table.
2. **PR 1's dual-write was live continuously** from the port-worker completion until this PR deploys. Any observed row created in that window exists in both tables.

After this PR's deploy, new observed rows land only in `slack_observed_users`. The legacy table's observed entries become a frozen snapshot of pre-PR-2 state — read by the UNION (under `source_priority=1`) for the same team_id they had before, and never updated again until PR 3 deletes them.
