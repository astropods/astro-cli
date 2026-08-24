# System audit findings in the admin console

## Summary

Nothing reports the states the write paths cannot fix on their own: an account
with no members, an owner who is not a member, a deployment wedged mid-rollout.
Finding one meant knowing to go looking. An hourly sweep now records them in a
table, and astro-queen renders them with acknowledgement.

## Design

A check is a name, a severity, and one SQL query returning `(subject_id,
subject_label, detail jsonb)`. Adding a check is adding an entry to that slice,
which is why the checks are data rather than functions.

The first cut covers five:

| Check | Severity | Flags |
|---|---|---|
| `account.no_members` | warning | A live account nobody can reach |
| `account.no_owner` | warning | No owner recorded, so ownership is underivable |
| `deployment.stuck_transition` | error | Pending, provisioning, deploying, or undeploying for over an hour |
| `cluster.config_stale` | error | Config never synced, or last synced over a day ago |
| `billing.unprovisioned` | warning | Account over a day old that the provisioning sweep still owes |

### Findings survive their fix

`system_audit_findings` holds one row per (check, subject). The sweep stamps
`last_seen_at` on everything it matches, then sets `resolved_at` on anything it
did not match this pass. A fixed problem stays visible as a resolved row for 30
days, so the console can answer "what was wrong last week", which a live query
cannot.

Acknowledgement marks a finding as triaged without fixing it, for the cases
where the answer is "we know, and we are choosing to leave it". A finding that
resolves and comes back is a new occurrence: it resets `first_seen_at` and drops
the acknowledgement, because deciding to ignore something once is not deciding
to ignore it forever.

### Sweep

`SystemAuditWorker` runs hourly on the maintenance queue with `RunOnStart`, so a
deploy refreshes the page immediately. One check failing logs and continues; the
worker returns the error afterwards so River retries the pass rather than
letting a broken check silently stop reporting.

### Console

`AdminService.ListAuditFindings` and `AcknowledgeAuditFinding` back a System
Audit page that groups by severity, shows each finding's detail payload, and
links account and deployment subjects to their existing admin pages.

## Migration

Apply `sql/astro-server/schema.sql` for `system_audit_findings` before deploying
astro-server. The first sweep runs on start and populates the page; no backfill.
