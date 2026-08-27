# Quota — per-account resource limits

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

Quota enforces per-account *counts* on a fixed set of resources (blueprints,
builds, deployments, members, knowledge stores, knowledge endpoints). It is
DB-backed, deliberately independent of billing, and identical for OSS and
hosted deployments.

## Boundary with billing

Quota and billing gate two different kinds of consumption, and neither
substitutes for the other:

- **Quota** enforces *resource counts*: how many blueprints, deployments,
  members, knowledge stores, or knowledge endpoints an account has right now,
  or how many builds it published this calendar month. The limit is a plain
  integer compared against a `COUNT(*)` query.
- **Billing** enforces *metered consumption*: compute time, knowledge-store
  storage/compute, and other usage that accrues continuously rather than
  counting rows. That gating lives in `apps/astro-server/internal/billing/**`
  and `internal/middleware/entitlement.go` — see
  [`billing-overview.md`](billing-overview.md).

A resource is one or the other, never both. `quota.IsResource` reports
whether a key is quota-managed; billing-gated features (compute, knowledge
storage) are not resources and have no `account_limits` row, so an admin
approving a quota-increase request whose `feature_key` isn't a managed
resource gets rejected in `admingrpc.ApproveQuotaIncrease` rather than
silently granted (see comment in `internal/quota/quota.go`'s package doc and
`internal/admingrpc/quota.go`'s guard). If you need to raise a billing limit,
that happens through the billing provider, not through this flow.

## Gated resources

Defined in `internal/quota/quota.go` (`AllResources`):

| Resource key | Counts | Query source |
|---|---|---|
| `blueprints` | Non-archived agents | `agents` |
| `agent_builds` | Version publishes in the current UTC calendar month | `agent_versions` |
| `agent_deployments` | Deployments in `pending`/`active` status | `deployments` |
| `members` | Account members | `account_members` |
| `knowledge_stores` | Knowledge stores not in `error` status | `knowledge_stores` |
| `knowledge_endpoints` | PrivateLink endpoints not in `error` status | `knowledge_store_endpoints` joined to `knowledge_stores` |

## Limit resolution

For a given `(account_id, resource)`, `DBChecker.effectiveLimit` resolves in
this order:

1. A per-account override row in `account_limits` (`limit_value`).
2. Else the system-wide default from `cfg.QuotaDefaults` (wired in
   `main.go`).
3. Else `Unlimited` (`-1`), meaning the check never blocks.

A limit of `0` disables the resource entirely and always blocks, regardless
of the `enforce` flag (`cfg.QuotaEnforce`) — this matches the older
"feature not in plan" behavior it replaced. When `enforce` is `false`,
over-limit usage is logged but not blocked; a disabled (`0`) resource still
blocks either way.

`DBChecker.Wrap` guards a handler on one or more resources and returns
`402 Payment Required` with a typed body (`FEATURE_NOT_IN_PLAN` for a
disabled resource, `ENTITLEMENT_LIMIT_REACHED` for an over-limit one) when
blocked; on a DB error it fails open and lets the handler run.
`DBChecker.WrapRegister` is the one wrapper with resource-specific logic: a
blueprint push only checks `blueprints` when the push would create a new
non-archived blueprint, so re-pushing an existing one is never blocked by the
blueprint cap even at capacity.

## Request/approval flow

An account holder without permission to raise their own limit files a
request; an astro-team admin resolves it from Queen.

1. **Request** — `POST /api/v1/accounts/:account/quota-increase`
   (`handlers.RequestQuotaIncrease`) inserts a row into
   `quota_increase_requests` with `status = 'pending'`. The handler rejects
   any `feature_key` that isn't `quota.IsResource` and requires a non-empty
   `reason`. `current_usage`/`current_quota` are client-reported snapshots
   for context, not re-derived server-side.
2. **List (self-service)** — `GET /api/v1/accounts/:account/quota-increase`
   (`handlers.ListQuotaIncreaseRequests`) returns the account's own requests,
   newest first, for the settings UI below.
3. **List (admin)** — `AdminService.ListQuotaIncreaseRequests`
   (`internal/admingrpc/quota.go`), optionally filtered by status, backs
   Queen's "Quota requests" admin page. See
   [`astro-queen.md`](astro-queen.md)'s feature table for the operator-facing
   UI; this doc stays canonical for the mechanism it calls into.
4. **Approve** — `AdminService.ApproveQuotaIncrease` requires a positive
   `grant_amount`. In one transaction it locks the pending row
   (`FOR UPDATE`), marks it `approved` with the grant amount, resolver, and
   note, and upserts `account_limits` with the grant as the new absolute
   limit (not an increment) for that resource. Approving a request for a
   non-quota-managed `feature_key` fails rather than silently granting
   anything — see the billing boundary above.
5. **Deny** — `AdminService.DenyQuotaIncrease` marks the row `denied` with a
   resolution note; no `account_limits` write happens.

Both approve and deny write an audit log entry (`auditlog.QuotaApprove` /
`auditlog.QuotaDeny`) when the account has an audit store configured.

## Frontend surface

`ResourceLimitsSection.tsx`
(`apps/astro-client/src/components/settings/ResourceLimitsSection.tsx`),
rendered from `UsageView.tsx`, is the account-facing quota UI:

- Fetches current usage/limits per resource from `GET
  /api/v1/accounts/:account/usage` (`useAccountUsage`, backed by
  `handlers.GetAccountUsage`, which reads through `quota.Reporter` — the same
  `DBChecker`, not a separate usage pipeline) and renders a meter grid (used
  / limit, `∞` for unlimited).
- Fetches the account's own quota-increase requests
  (`useQuotaIncreaseRequests`) and renders them in a status table
  (pending/approved/denied, styled via `StatusBadge`).
- `RequestIncreaseDialog.tsx` is the request form: pick a feature (or a fixed
  one is preset), see current usage/quota, optionally propose an amount,
  and give a required reason. Submission calls
  `useRequestQuotaIncrease`, which invalidates the requests query on success.
- `canRequestIncrease` is a prop passed down from `UsageView.tsx`; when false,
  the section is read-only (no button, no dialog).
- `meterMeta` in `RequestIncreaseDialog.tsx` is the display-label map for the
  six resource keys; it's a UI concern only; the resource identifiers
  themselves live server-side in `quota.AllResources`.

## Verify

- `go test ./internal/quota/...` (unit tests: limit resolution, override vs.
  default, `enforce` flag, `WrapRegister`'s blueprint-exists branch, 402 body
  wording).
- `go test ./handlers/... -run QuotaIncrease` (request/list handler tests).
- `go test ./internal/admingrpc/... -run QuotaIncrease` (approve/deny
  transaction behavior, including the non-managed-resource rejection).
- `cd apps/astro-client && bun x vitest run src/components/settings/ResourceLimitsSection.test.tsx src/components/RequestIncreaseDialog.test.tsx`
