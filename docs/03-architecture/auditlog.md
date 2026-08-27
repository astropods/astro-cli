# Audit log

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

The audit log is the user-facing record of who did what on an account: every
member-visible mutation (deploy, undeploy, member/role changes, avatar
changes, agent visibility, access-group changes, and more) is written to a
single `audit_logs` table and shown back to account members in Settings. It
is not the same system as the internal ops health-check sweep described in
[`systemaudit.md`](systemaudit.md) — that system produces *findings* about
the platform's own state (stuck deployments, stale cluster config), not a
trail of member actions, and it is surfaced in astro-queen, not in the
customer-facing app.

This doc covers `apps/astro-server/internal/auditlog/**`, the handler at
`apps/astro-server/handlers/auditlog.go`, and the frontend view. The broader
"Account & org settings UI" area-map row's glob
(`apps/astro-client/src/pages/settings/**`, `apps/astro-client/src/components/settings/**`)
also matches the audit-log frontend files; this doc is the canonical source
for the audit-log feature specifically, that row stays canonical for the
rest of `settings/`.

## Data model

One table, `audit_logs` (`sql/astro-server/schema.sql`):

```
id, account_id, actor_id, actor_type, action,
resource_type, resource_id, resource_name, description,
metadata (jsonb), ip_address, user_agent, created_at
```

- `actor_type` is one of `user`, `admin`, or `system` (`auditlog.ActorType`).
  An `admin` entry comes from an operator action taken through astro-queen's
  admin gRPC API (`auditlog.ForAdmin`, actor ID `admin:grpc`) — for example,
  an operator force-resuming billing or deleting a defunct account shows up
  in that account's own audit log, tagged as an admin action, alongside the
  account's own member activity.
- `action` follows a `<resource>.<verb>` naming convention, defined as
  constants in `internal/auditlog/actions.go` (`account.rename`,
  `deployment.deploy`, `member.add`, `access_group.create`, and so on).
- `metadata` is a free-form JSONB blob set by the caller; most call sites
  leave it empty and rely on `resource_name`/`description` instead.
- Six indexes back the query patterns: by account+time
  (`idx_audit_logs_account_created`), by account+resource type+time
  (`idx_audit_logs_account_resource`), by actor+time (`idx_audit_logs_actor`),
  a plain created_at index (`idx_audit_logs_created`), a covering index for
  the "latest audit entry per resource" lookups
  (`idx_audit_logs_resource_latest`), and a creator/action/resource lookup
  index (`idx_audit_logs_creator_lookup`).

## Writing an entry

`auditlog.Store.Log` does a synchronous insert; `LogAsync` fires it in a
goroutine and only logs a failure, for call sites where the audit write
shouldn't block the response. `LogAsync` is nil-receiver safe, so a handler
can hold a possibly-nil `*auditlog.Store` without a nil check at every call
site.

Handlers build the `Event` with two helpers in `internal/auditlog/helpers.go`:

- `auditlog.FromGinContext(c, accountID)` — for a user-initiated HTTP request,
  fills actor ID from the authenticated user, IP, and User-Agent from the
  request.
- `auditlog.ForAdmin(accountID, adminIdentity)` — for an admin gRPC action,
  sets `actor_type: admin` and `actor_id: "admin:" + adminIdentity`.

The caller sets `Action`, `ResourceType`, `ResourceID`, and any
`ResourceName`/`Description`/`Metadata` after building the base event.

Deployment, account, member, invitation, access-group, avatar, quota,
payment method, M2M app (create/update-scopes/delete, secret create/delete),
and cache-invalidation handlers all write through this path
(`handlers/deploy.go`, `handlers/accounts.go`, `handlers/org.go`,
`handlers/access_groups.go`, `handlers/avatar.go`, `handlers/experiments.go`,
`handlers/transfer.go`, `handlers/agents.go`, `handlers/apps.go`,
`handlers/deployment_access_management.go`, `handlers/user_deployments.go`,
`handlers/ingestion.go`, `handlers/payment_methods.go`), as do several admin
gRPC handlers (`internal/admingrpc/accounts.go`, `billing.go`, `cache.go`,
`imagecache.go`, `quota.go`, `server.go`). See `actions.go` for the full
current action list; treat it as the source of truth over this doc, since new
actions get added there without necessarily updating prose elsewhere.

### Observers: how the watcher feature reuses the audit trail

`Store.Observe` registers an `Observer` (`OnAudit(ctx, Event)`) that runs
after every successful write. `internal/watcher/observer.go`'s
`AuditObserver` is the one production observer: it enrolls the actor of a
qualifying deployment action as a watcher (someone who gets notified about
that deployment) purely by hooking the audit write, so any handler that
already audits a deployment mutation gets watcher enrollment for free instead
of needing a second, easy-to-forget call site. Observers are advisory — an
observer failure is logged and does not fail or undo the audit write itself.

## Querying and pagination

`Store.Query` takes `QueryParams` (account ID plus optional actor, resource
type, resource ID, action, and cursor) and returns entries newest-first.
Pagination is cursor-based on `(created_at, id)` DESC, not offset-based:

- The cursor is `created_at` plus `id` as a tie-breaker for entries with the
  same timestamp (`ParseCursor`/`FormatCursor` encode/decode it as
  `"<RFC3339Nano-timestamp>,<id>"`).
- `Query` fetches `limit + 1` rows to detect `has_more` without a separate
  count query, and the handler trims the extra row before responding.
- Limit defaults to 50, capped at 200 (`ParseLimit`).

`Store.Filters` returns the distinct `resource_type` and `action` values seen
for an account, powering the two filter dropdowns in the UI. A handful of
other `Store` methods (`LatestPerResource`, `LatestPerResourceByAction`,
`LatestPerResources`, `BulkDistinctActorsFor`, `DistinctActorsFor`) exist
for other features to derive "who last touched this" or "who has ever acted
on this" from the audit trail rather than tracking it separately — for
example, the deployments list surfaces "who deployed" this way.

## Tenant isolation

Every `Store.Query`/`Filters`/`Latest*` method takes an account ID (or a
bounded set of account IDs for the cross-account `LatestPerResources`
variants) and includes it as a hard `WHERE` clause; there is no method that
queries across all accounts unscoped. This is exercised directly, not just
assumed: `internal/auditlog/store_test.go` covers the query-builder logic
against a mocked DB, and `e2e/audit_log_test.go` includes
`TestAuditLog_AccountIsolation` and
`TestAuditLog_LatestPerResource_AccountIsolation`, which insert rows for two
accounts and assert a query scoped to one never returns the other's rows,
against a real Postgres instance.

On the frontend, `useAuditLog` (`apps/astro-client/src/api/queries/auditlog.ts`)
deliberately does not set `placeholderData: keepPreviousData` on its
`useInfiniteQuery`, even though that's the normal pattern elsewhere: keeping
one account's entries on screen while another account's first page is still
loading (for example, right after switching orgs) would be a client-side
cross-tenant leak, however brief.

## Who can view it

Both listing routes sit behind the same account-scoped middleware chain:

```
GET /api/v1/accounts/:account/audit-log
GET /api/v1/accounts/:account/audit-log/filters
```

```
accountSettings := protected.Group("/accounts/:account")
accountSettings.Use(middleware.ResolveAccount(accountStore))
accountSettings.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
```

`RequireAccountPermission` branches on account type: for a personal account,
any member (in practice, just the owner) passes; for an organization
account, the caller's session must be scoped to that org (via switch-org)
*and* the JWT's permissions claim must include `org:manage`. There's no
separate "audit log viewer" role — viewing the log requires the same
`org:manage` permission as other account-settings actions. The frontend
gates both the personal-account view (`AuditLogSettings.tsx`) and the
org-settings view (`OrgAuditLogSettings.tsx`) through the same
`AuditLogView` component and the same backend routes; only the `account`
value passed in (personal account name vs. org slug) differs.

## Known gaps

- `metadata` is optional and most call sites don't populate it, so several
  audited actions carry no more detail than their `resource_name`/
  `description` already gives.
- There's no dedicated retention/purge job for `audit_logs` (contrast with
  `systemaudit`'s 30-day purge of resolved findings) — rows accumulate
  indefinitely.
