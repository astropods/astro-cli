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

The first cut covers six:

| Check | Severity | Flags |
|---|---|---|
| `account.no_members` | warning | A live account nobody can reach |
| `account.no_owner` | warning | No owner recorded |
| `account.owner_not_member` | error | The recorded owner is not a member of the account |
| `deployment.stuck_transition` | error | Pending, provisioning, deploying, or undeploying for over an hour |
| `cluster.config_stale` | error | Config never synced, or last synced over a day ago |
| `billing.unprovisioned` | warning | Account over a day old that the provisioning sweep still owes |

The two ownership checks matter more now that `accounts.owner_user_id` decides
who owns an account rather than deriving it from WorkOS. Nothing repairs an
ownerless account automatically, by design, so the report is how one gets found.
`account.owner_not_member` is an error because it names the invariant the column
is heading toward: once the composite foreign key to `account_members` lands,
a row like that stops being reportable and starts being rejected.

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

### Two membership reads that stopped at one page

Manager notifications and the member list each asked WorkOS for 100 memberships
and treated the result as the whole organization. In an org larger than that,
managers past the cursor were never notified and members past it rendered with
no role. Both now use `ListAllMemberships`, which follows the cursor to the end.

The comment on the notification lookup said "managers are few; one page of 100
covers any real org". Managers are few; memberships are not, and the page limit
counts memberships.

## Migration

Apply `sql/astro-server/schema.sql` for `system_audit_findings` before deploying
astro-server. The first sweep runs on start and populates the page; no backfill.
