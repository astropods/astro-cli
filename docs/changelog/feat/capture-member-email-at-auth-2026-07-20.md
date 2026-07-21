# Retire the WorkOS events poller; keep auth-time email capture

## Summary

We polled the WorkOS Events API every 15s and reconciled inbound changes into our own state — organizations, memberships, and mirrored member emails. That made WorkOS a second source of truth that could patch accounts and orgs behind the server's back. Our server is the source of truth: all account, org, and membership changes flow through it. This change removes the inbound poller so state is only ever mutated through our own write paths.

Member emails were the one thing the poller wrote that we still need (they power dev-tool telemetry attribution). Those are now captured at auth time instead.

## Design

Two parts:

**Capture member email at auth time.** The session user returned by WorkOS authentication already carries `id`, `email`, and `email_verified`, so we reuse the existing `memberemails.Store.UpsertWorkOS` (source `workos`) — no store or schema changes. Best-effort upserts (log-and-continue, never fail the request) run in the login callback (primary writer: every user, every login) and in account create (the creator's email). A single `memberemails` store instance is shared across both handlers.

**Remove the events poller.** The `workos.events` River job, its worker, its single-worker `workos` queue, and the `EventsConsumer` that processed organization/membership/user events are deleted. The periodic `MemberEmailReconcile` backfill is kept — it queries WorkOS directly for members still missing an email and is now purely a safety net behind auth-time capture (moved off the deleted `workos` queue onto the default queue).

Net effect: nothing polls WorkOS for change events. Org/membership state changes only via the server's write paths; member emails are captured at auth time and backfilled by reconcile.

The `member_emails` table (plus its constraints and indexes) is renamed to `account_member_emails` for naming consistency with the other `account_*` tables. Only the table name changes — columns, semantics, and the `memberemails` package are unchanged. The sibling `member_email_reconcile_attempts` table is left as-is.

## Migration

No API, request/response, or configuration changes. `WORKOS_API_KEY` is still used (auth + reconcile).

Schema: applying the SDL renames `member_emails` → `account_member_emails`. Atlas should apply this as a rename; if it instead drops and recreates the table, the mirror is repopulated automatically (auth-time capture on next login, reconcile backfill) so no manual action is needed. The now-unused `workos_event_cursor` and `workos_event_errors` tables (poller state) are dropped.
