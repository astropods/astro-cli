# Local mirror of member emails

## Summary

External dev-tool telemetry (Claude Code, and soon others) is stamped with the developer's `user.email`, but attributing that spend to an Astro member requires mapping the email to a member's `user_id` — and WorkOS is the authority for the corporate email. Resolving that per request means a WorkOS `GetUser` fan-out over the account's members, which is slow at enterprise scale and hammers a third-party API on the page path. This PR mirrors member emails into our own database so the mapping becomes a single indexed lookup. It's preparatory: no surface consumes it yet — the dev-tool Insights people roll-up will.

## Design

**`member_emails` table.** A local mirror keyed by WorkOS `user_id`, one-to-many (a user may have several emails), with a `source` column (`workos` today) and a `verified` flag. The one-to-many shape plus `source` leave room to add arbitrary emails directly later (e.g. a user-added address that isn't in the IdP) without a schema change; that flow is intentionally not implemented here. `email` is unique and stored lowercased, so the attribution lookup is unambiguous and case-insensitive.

**Ongoing sync via the existing WorkOS events poller.** WorkOS `user.created`/`user.updated` events carry the email inline, so `processUserEvent` (previously a no-op for the no-local-user-table case) now upserts it — no extra WorkOS call. Upsert replaces the user's prior `workos` email so a change of primary email doesn't leave a stale row; other sources are untouched (a partial unique index enforces at most one `workos` email per user). The mirror write is **best-effort** — a failure is logged, never returned — because the poller processes events strictly in order and must not be wedged by a non-critical mirror. A dropped write for a member who has no email yet is repaired by the reconcile job; a dropped write on an email *change* leaves the prior email until that member's next `user.updated` (a narrow window — the write only fails on a transient DB error). `user.deleted` clears the user's emails alongside the existing membership cleanup.

**Backfill + self-heal via a periodic reconcile job.** Events only flow forward from the cursor, so a bounded River job resolves members that have no recorded email via `GetUser`, capped per run with deterministic paging so a first backfill drains over several runs rather than one burst. A member with no resolvable email is recorded and backed off so the job doesn't re-query them every run; transient WorkOS failures (e.g. rate limits) just retry on the next run. It runs on the default worker queue — not the single-worker events-poller queue — so a backfill never stalls event processing; once drained it does near-zero work.

**Attribution join (future consumer).** `EmailsForAccount` returns `email → user_id` for an account's members by joining `member_emails` to `account_members`. This is what will replace the per-request WorkOS fan-out in the dev-tool Insights people roll-up.

## Migration

None required. Additive table applied by Atlas; the reconcile job backfills existing members automatically on rollout. When WorkOS isn't configured, the sync and reconcile jobs are simply not scheduled.
