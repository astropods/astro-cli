# Slack user attribution: traces & Insights round-trip

## Summary

Slack messages now flow with the right user identity through the trace pipeline to Insights. Before this change, every Slack message tagged the Langfuse trace with the raw Slack `U07…` id regardless of whether the sender had linked their identity. Two visible failure modes:

1. **Members who linked their Slack identity** were mis-attributed as a `U07…` row instead of their Astro identity, even though the server *did* resolve them internally.
2. **Slack users without a link** disappeared into a single aggregated "Unidentified" bucket — a CEO scanning Insights couldn't see *which* Slack teammates were driving spend.

This change makes the resolved identity flow from the server, through the messaging container, onto the trace, and into Insights — without splitting any human across multiple rows.

## Design

```
Slack event
  → messaging adapter → POST /deployments/authorize
       (response carries: user_id, slack_user_id, slack_team_id)
  → pb.Message.User.Id =
       result.UserID    if WorkOS link exists
       slackUserID      otherwise (raw "U07ABCDEF" — same shape every
                                   historical trace already had)
  → agent SDK plumbs msg.User.Id → langfuse.user.id  (unchanged wiring)
  → /authorize live-ingest writes (team_id, slack_user_id) to
    slack_identity_mappings (observed-only directory rows)
  → Insights users-summary joins on slack_identity_mappings:
       - linked rows → merge bare-Slack metrics into the WorkOS user row
       - observed-only rows → stamp slack_team_id for the slack:// deep link
```

### Server: resolve before the anyone-grant short-circuit

`apps/astro-server/handlers/authorization.go` used to short-circuit on an `anyone` grant *before* `resolveCandidates`. Most Slack deployments ship with `slack: anyone`, so resolution almost never fired in practice — the response always came back with no `user_id`. The handler now resolves the principal first, then checks the anyone grant. Resolution is one indexed lookup on `slack_identity_mappings` (sub-ms).

Response shape grows by two fields when `identity_type=slack`:

```json
{
  "allowed": true,
  "user_id": "user_01H…",      // resolved WorkOS user (when linked)
  "slack_user_id": "U07ABC…",  // echoed input (always when allowed)
  "slack_team_id": "T07XYZ…"   // echoed input (always when allowed)
}
```

Identity fields are only surfaced on `allowed=true` — denials don't leak mapping state.

### Server: slack_identity_mappings doubles as a Slack user directory

`slack_identity_mappings` previously stored only oauth-linked users (`workos_user_id NOT NULL`). The schema now allows `workos_user_id IS NULL` for **observed-only** rows (`source='observed'`) — directory entries that record we've seen a Slack identity without anyone linking it yet. A CHECK constraint enforces the oauth/observed split: oauth rows must have a workos_user_id, observed rows must not.

Two write paths populate observed rows:

- **Live ingest**: every `/authorize` call for a Slack identity fires an async UPSERT (per-process in-memory dedupe keeps a chatty workspace from burning DB writes). New users get directory entries on their first message.
- **Backfill (River job)**: a periodic worker (`SlackDirectoryBackfillWorker`, runs daily with `RunOnStart=true` so it fires on first deploy) iterates every account with at least one linked Slack member, queries Langfuse for that account's distinct bare-Slack userIds, and upserts them with the account's primary team_id. Idempotent — re-runs are no-ops.

Single-workspace orgs (the common case) get unambiguous team attribution. Multi-workspace orgs get best-effort attribution; the rare cross-workspace user-id collision is acceptable.

### Server: users-summary enriches and merges

`GET /observability/users-summary` now does a directory join after the Langfuse aggregation:

- **Linked entry** (Slack id → WorkOS id) → the bare-Slack row's metrics merge into the WorkOS user's row. Cost/requests/tokens sum exactly; last_seen takes max; agents_used unions on `(account, name)`. Bob's pre-link Slack spend and post-link Astro spend collapse to one row keyed by his WorkOS id, rendered as his named-member identity.
- **Observed-only entry** (Slack id with team_id, no WorkOS link) → `slack_team_id` is stamped on the response row so the frontend can build the `slack://user?team=…&id=…` deep link.

The maxUsersInResponse cap and final cost-desc sort happen *after* the merge — truncating first could drop a bare-Slack row that should have rolled into a top-N WorkOS row.

### Messaging: bare slack id on `User.Id`, never namespaced

`canonicalUserID()` writes the resolved WorkOS id for linked users; otherwise the raw slack id (`U07ABCDEF`). Critically, **no namespacing** — every Slack user has one aggregation key in Langfuse forever (same shape every historical trace already carried), so the People view aggregates pre-PR and post-PR traffic from the same human into one row. The earlier draft of this PR namespaced unlinked users as `slack:T:U`; that created two keys per human (historical bare + new namespaced) and split spend across two rows. Dropping namespacing is what makes the server-side merge produce clean numbers.

### Insights UI: per-id Slack rows with deep link

`apps/astro-client/src/components/activity/`:

- `user-classification.ts` exposes `isSlackUserId` matching `U + 8..11 uppercase alphanumerics`. Tighter than necessary would drop real Slack users; looser would false-positive on arbitrary 5-char `U…` strings emitted by custom SDKs. The previously-supported `slack:<team>:<user>` form is gone since the messaging adapter no longer emits it.
- `TopSpendersTable.tsx` (users view) reads `slack_team_id` from the response row (server-attached via the directory join) and builds the deep link as `slack://user?team={team}&id={uid}`. Rows without `slack_team_id` (tombstoned users pre-backfill) render as plain text — clicks don't navigate to a broken Slack URL.
- `UsersUsedAvatars.tsx` (agents view's People column) renders Slack ids with the Slack icon + "Slack user - U07…" label, matching the Insights People table.

Empty `user_id` keeps falling into "System spend" (Unattributed). The Unidentified bucket now describes what actually lands there post-PR: arbitrary non-WorkOS, non-Slack trace ids from custom integrations or direct SDK calls.

## Migration

- **Schema**: `workos_user_id` on `slack_identity_mappings` is now nullable, with a CHECK enforcing the oauth/observed split. Atlas diff applies automatically on next deploy.
- **Historical traces**: keep their bare `U07…` user_ids in Langfuse — the new `isSlackUserId` recognizes them, the directory backfill seeds team_ids for the deep link, and the merge logic rolls them into the WorkOS row for any user who's since linked.
- **No frontend backfill needed**: the page learns to interpret existing data correctly on next load post-deploy.
- **Slack adapter wire format**: `msg.User.Id` now carries `user_…` (linked WorkOS user) or `U07ABCDEF` (unlinked Slack user) or empty (anonymous). Agents that previously read `platform_data["slack_user_id"]` should read `msg.User.Id` directly — the `user_` prefix is the discriminator.

## Backfill (Slack user directory)

The historical backfill is a **one-shot River job** — `SlackDirectoryBackfillWorker` — that runs automatically on the first deploy of each environment and never runs again. After that, the directory keeps itself current via the `/authorize` live-ingest path (every Slack message upserts the `(team_id, slack_user_id)` pair).

### How the "never runs again" guarantee works

Three layers of protection, any one of which would be sufficient:

1. **`main.go` enqueue gate** — before enqueueing the job, queries `slack_directory_backfill_marker`. If the row exists, the job is never enqueued. So after the first successful run, every subsequent pod restart adds zero River jobs.
2. **Worker entry gate** — `Work()` re-checks the marker on entry and exits immediately if set. Belt-and-suspenders against a race where the marker was written between the main.go check and the job actually executing (e.g. two replicas booting simultaneously).
3. **River `UniqueOpts{ByArgs: true}`** — collapses concurrent enqueue attempts (one per replica during a rolling restart) into a single queued job.

In practice: the job runs **at most once per environment**, end of story.

### What it does

- Lists every `(account_id, team_id)` pair derivable from `slack_identity_mappings` — accounts with at least one linked Slack member.
- For each, queries Langfuse for every distinct user_id matching the bare-Slack shape (`^U[A-Z0-9]{8,11}$`).
- Upserts each `(team_id, slack_user_id, source='observed')` row via `UpsertObserved`. Active rows (oauth or observed) stay untouched; revoked rows revive in-place.
- Writes the marker row to `slack_directory_backfill_marker`.

Accounts with zero linked Slack members are skipped — there's no `team_id` signal to attribute their bare-Slack userIds to. They'd need either (a) an admin to link Slack, after which any new message updates the directory via live-ingest, or (b) a separate workspace-discovery path (out of scope for v1).

### Verifying it ran

After deploying to preview, in the query console:

```sql
SELECT completed_at FROM slack_directory_backfill_marker;
-- Expect one row with the timestamp of first deploy.
```

Per-account row count:

```sql
SELECT
  a.name AS account,
  COUNT(*) FILTER (WHERE sim.source = 'observed') AS observed_rows,
  COUNT(*) FILTER (WHERE sim.source = 'oauth')    AS oauth_rows
FROM slack_identity_mappings sim
JOIN accounts a ON a.id = ANY(
  SELECT am.account_id
  FROM account_members am
  JOIN slack_identity_mappings linked
    ON linked.workos_user_id = am.user_id
   AND linked.team_id        = sim.team_id
   AND linked.revoked_at IS NULL
)
WHERE sim.revoked_at IS NULL
GROUP BY a.name
ORDER BY observed_rows DESC;
```

Then spot-check in the UI: reload Insights → People view → click a "Slack user - U07…" row → Slack opens that user's profile.

### Re-running (rare)

If you ever need to re-run the backfill — e.g. after onboarding a new account that had pre-existing Slack history before linking — delete the marker:

```sql
DELETE FROM slack_directory_backfill_marker;
```

The next pod restart will enqueue the job, run the whole sweep again, and re-write the marker. The work is idempotent (UPSERT-based) so existing rows aren't disturbed.

### Rollback

Directory rows are pure additive metadata. To wipe the backfill without touching any linked (oauth) data:

```sql
UPDATE slack_identity_mappings
SET revoked_at = now(), updated_at = now()
WHERE source = 'observed' AND revoked_at IS NULL;

-- And clear the marker so a future re-run is possible:
DELETE FROM slack_directory_backfill_marker;
```

The deep-link affordance disappears for unlinked users; live-ingest will refill the directory from new traffic over the following days.
