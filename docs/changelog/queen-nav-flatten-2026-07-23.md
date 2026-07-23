# Account detail: billing & observability health + repair

## Summary

The account detail view surfaced billing/observability IDs but couldn't tell an
admin whether they were set up correctly, nor fix them. This adds live health
checks and idempotent repair actions for the per-account billing and
observability integrations, and removes a too-easy destructive control.

## Design

All new reads/writes hang off the admin gRPC service and the account detail
page; the billing/observability provisioners are injected into the admin server
via setters (mirroring the existing quota-reporter wiring), staying nil — and
reporting "not configured" — when a backend is disabled.

- **Metronome ingest aliases.** A live check compares the customer's actual
  ingest aliases against the expected set (`{account_id, bifrost_customer_id}`),
  rendered as an Alias/Expected/Present table with missing rows flagged. A
  **Recover** button rewrites the expected aliases; a **Register** button creates
  the Metronome customer (seeded with the aliases) when the account has none.
  The alias check is a separate on-demand call so the detail page stays DB-only
  and fast.
- **Langfuse & Bifrost.** Each observability row shows its ID or a **Recover**
  button when missing. Recover reuses the existing idempotent provisioning —
  `langfuse.Provisioner.EnsureProject` and a new one-line exported
  `aigateway.Provisioner.EnsureCustomer` (which also re-syncs the Metronome
  alias on a fresh mint). A new `billing.BillingProvider.GetIngestAliases`
  read method backs the alias check.
- **Rename is now guarded.** The one-click header rename is gone; renaming lives
  only in the account's Maintenance section behind a reveal + a destructive
  confirm dialog, since the name is referenced in URLs and integrations.
- Recover/register mutations write the returned ID straight into the cached
  account so the row refreshes immediately, then invalidate to reconcile.

## Migration

None. All endpoints are additive and read/write existing tables; no schema
changes.

---

# Jobs: history per worker

## Summary

The Jobs page had a standalone History tab that dumped a global, weakly-scoped
job list. It's replaced with history scoped to the worker you're looking at.

## Design

- **History tab removed.** Tabs are now Overview · Workers · Running.
- **Per-worker history.** Each Workers row expands into that kind's job history —
  state filter, paginated list, retry/cancel, and expandable args/errors.
- **Deep-links preserved.** `?job=<id>` resolves the job's kind, opens the
  Workers tab, auto-expands the owning worker, and highlights the row.
- Completed jobs no longer show a Retry action (retry is for discarded/cancelled).

## Migration

None. UI-only change.

---

# Reconcile: member email backfill runs daily

## Summary

`workos.member_email_reconcile` was firing every 10 minutes. Emails are already
captured at auth time; this job is only the safety net for gaps, so a 10-minute
cadence was needless load on WorkOS and the queue.

## Design

Interval and uniqueness window both moved from 10m to 24h; `RunOnStart` is kept
so a fresh boot still reconciles once immediately, then daily thereafter.

## Migration

None.
