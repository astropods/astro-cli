# System audit (ops health checks)

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

System audit is an hourly, in-process sweep of SQL health checks over the
platform's own state: stuck deployments, stale cluster config, accounts
whose soft-delete purge is overdue, accounts never provisioned for billing.
Each check is a plain SQL query; a row it returns becomes a *finding*, held
in a single Postgres table with an open → (optionally acknowledged) →
resolved lifecycle. Findings are surfaced to operators in astro-queen.

This is a distinct system from [`auditlog.md`](auditlog.md), despite the
similar name: `auditlog` records member-initiated actions on an account for
that account's own users to review; `systemaudit` finds problems with the
platform's internal state, for operators, and has no per-account visibility
or user-facing surface at all. Neither reads from the other.

Covers `apps/astro-server/internal/systemaudit/**`,
`apps/astro-server/internal/riverqueue/system_audit.go`, and
`apps/astro-server/internal/admingrpc/audit_findings.go`. For how
astro-queen presents findings (the Audit findings page, its acknowledge
button), see the "Audit findings" row of
[`astro-queen.md`](astro-queen.md)'s feature table — this doc stays
canonical for the checks themselves and the finding lifecycle, not Queen's UI.

## The checks

`internal/systemaudit/checks.go` defines the check list as data
(`[]Check{Name, Severity, Title, Query}`), run in order by
`Store.Run`. As of this writing there are four:

| Check name | Severity | Detects |
|---|---|---|
| `account.purge_overdue` | error | A soft-deleted account whose retention window plus a one-day grace period has passed but hasn't been purged. Exists because the purge sweep skips an account it can't finish and reports success anyway, so a permanently stuck purge is otherwise invisible. |
| `deployment.stuck_transition` | error | A deployment in `pending`, `provisioning`, `deploying`, or `undeploying` whose `status_changed_at` is more than an hour old. |
| `cluster.config_stale` | error | A cluster whose `config_synced_at` is null or older than 24 hours. |
| `billing.unprovisioned` | warning | An account older than 24 hours with no `billing_provisioned_at` set. |

Each query must return exactly `subject_id`, `subject_label`, and a `detail`
jsonb column; `TestChecksAreValidSQL` in
`internal/systemaudit/store_integration_test.go` runs every check's query
against a real database to catch a broken query (wrong column name, bad
join) before it ships. A third severity, `info`, is defined
(`SeverityInfo`) but no shipped check currently uses it.

Adding a check means appending to the `checks` slice; there's no plugin
registration or separate config, so the list in `checks.go` is authoritative
over any summary here.

## Finding lifecycle

Findings live in `system_audit_findings`, keyed on `(check_name, subject_id)`:

```
check_name, subject_id, subject_label, severity, detail,
first_seen_at, last_seen_at, resolved_at, acknowledged_at
```

Each hourly run, per check, `Store.runCheck`:

1. Runs the check's query and upserts one row per result row
   (`ON CONFLICT (check_name, subject_id) DO UPDATE`), refreshing
   `subject_label`, `severity`, `detail`, and `last_seen_at`.
2. Resolves every row for that check whose `last_seen_at` is older than this
   run's start time and isn't already resolved — i.e., anything not returned
   by this run's query that was still open.

There is no separate "resolve" action anywhere in the code: resolution is
always automatic, driven purely by whether the underlying condition still
matches on the next hourly pass. `AcknowledgeAuditFinding` (the only mutating
admin gRPC RPC on this system) sets `acknowledged_at` and nothing else —
acknowledging a finding doesn't stop it from auto-resolving once the
condition clears, and doesn't stop it from reopening if the condition
reappears later.

If a finding that was previously resolved reappears, the upsert's `CASE`
logic treats it as new: `first_seen_at` resets to now and `acknowledged_at`
resets to null, rather than treating it as a continuation of the old
finding. So the practical lifecycle is:

```
open --(condition clears next run)--> resolved
open --(operator acks, condition persists)--> open, acknowledged
open/acknowledged --(condition clears next run)--> resolved (ack cleared)
resolved --(condition reappears)--> open (fresh first_seen_at, no ack)
```

A separate purge step (`Store.Purge`, called at the end of every run from
`SystemAuditWorker`) hard-deletes findings that have been resolved for more
than 30 days (`auditFindingRetention` in `system_audit.go`). Open or
acknowledged-but-unresolved findings are never purged.

## Scheduling

`SystemAuditWorker` runs as a River periodic job
(`internal/riverqueue/periodic.go`), registered with
`river.PeriodicInterval(time.Hour)` and `RunOnStart: true`, so a fresh
deploy runs one sweep immediately instead of waiting up to an hour for the
first pass. It runs every check via `Store.Run`, logs one summary line
(`system audit: completed`, with `open`/`resolved`/`purged` counts), and
purges resolved findings older than the retention window. A single check's
query error is logged and does not stop the other checks in that pass, but
does propagate as the job's own error (so River sees the job as failed, even
though most checks may have succeeded) — see `SystemAuditWorker.Work`.

## Reading findings

`admingrpc.ListAuditFindings` lists findings (optionally including resolved
ones), ordered by severity (`error` before `warning` before `info`) then
most-recently-seen first, and rolls up open counts by severity
(`OpenErrors`, `OpenWarnings`) for a summary badge. `AcknowledgeAuditFinding`
requires both `check_name` and `subject_id`, and returns `NotFound` if there
is no open (unresolved) finding matching them — you cannot acknowledge an
already-resolved finding.

There's no per-account scoping on any of this: any operator with admin gRPC
access sees findings across every account and cluster, matching the rest of
the admin gRPC surface (see [`astro-queen.md`](astro-queen.md)'s auth model:
mTLS is all-or-nothing, there's no per-operator identity or role).

## Known gaps

- No automated test exercises `SystemAuditWorker.Work` or
  `admingrpc.ListAuditFindings`/`AcknowledgeAuditFindings` directly; only
  `internal/systemaudit/store_integration_test.go`'s two tests
  (`TestChecksAreValidSQL`, `TestRunRecordsResolvesAndReopens`) cover this
  system, both requiring a real Postgres instance (`//go:build integration`).
- `SeverityInfo` has no consumer yet.
