# Quota — per-account resource limits

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-31

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
approving a quota-increase request whose `feature_key` isn't requestable
gets rejected in `admingrpc.ApproveQuotaIncrease` rather than silently
granted (see comment in `internal/quota/quota.go`'s package doc and
`internal/admingrpc/quota.go`'s guard). If you need to raise a metered
billing limit, that happens through the billing provider, not through this
flow.

### The one exception: the spend-limit ceiling

`spend_limit` (`quota.KeySpendLimit`) is a requestable key that is *not* a
resource. It carries no count and nothing enforces it here; it exists so the
one billing number a customer can raise on its own travels the same
request → review → grant pipeline as a resource count, instead of an email.

`quota.IsRequestable` is the wider predicate the request and approve paths
use: every resource, plus `spend_limit`. `quota.IsResource` is unchanged, so
the checker and the usage report never see the key.

What a grant means: `account_limits` holds the **ceiling** the account may
set its own monthly spend limit to, in whole dollars, not the limit itself.
Approving raises what the account is allowed to choose; the account still
picks a number under it, and nobody is charged more by the approval.
`quota.SpendCeilingUSD` resolves it — a grant when one exists, else
`billing.MaxSelfServeSpendUSD` ($1,000). A grant never *lowers* the ceiling,
which also makes the `-1`/`0` sentinels resource limits carry read as the
default here rather than as no spend at all.

Two places read the ceiling, and both must, or a grant only half-lands:

- `handlers.SetBillingSpendThresholds` bounds what the account may write.
- `riverqueue.BillingGatewayBudgetWorker.ceilingUSD` clamps the AI gateway
  budget. Without this the gateway would keep refusing spend the billing
  provider already accepted.

An operator can also move both numbers directly, without a request:
`AdminService.SetAccountSpendLimit` writes the limit and raises the ceiling to
match. See [`astro-queen.md`](astro-queen.md)'s "Setting an account's spend
limit".

See `internal/quota/spend_limit_test.go` and
`internal/admingrpc/quota_spend_limit_integration_test.go` (the request →
approve → raised-ceiling chain, real Postgres, `-tags integration`).

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
   any `feature_key` that isn't `quota.IsRequestable` and requires a
   non-empty `reason`. `current_usage`/`current_quota` are client-reported
   snapshots for context, not re-derived server-side. A `spend_limit`
   request carries two extra rules, because a reviewer cannot read the
   figure off a count: `requested_amount` is required, and it must exceed
   the account's current ceiling (an amount at or under it is already the
   account's to set).
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
   limit (not an increment) for that resource, or the new ceiling for
   `spend_limit`. Approving a request for a `feature_key` that isn't
   requestable fails rather than silently granting anything — see the
   billing boundary above.
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
  six resource keys plus `spend_limit`; it's a UI concern only; the resource
  identifiers themselves live server-side in `quota.AllResources`. A
  `money: true` entry switches every amount in the dialog to currency and
  makes the requested amount required.

The spend-limit request is reached from the number it is about, not from this
section: `ManageLimitsDialog.tsx` (Settings → Billing) blocks a limit above
`spend_ceiling` from `GET /billing/spend` and offers "Request an increase",
which swaps the limits dialog for `RequestIncreaseDialog` with
`featureKey="spend_limit"`. The feature picker in this section stays
resource-only, since it offers what the usage endpoint meters. Approved or
pending spend-limit requests still appear in the requests table here,
formatted as currency.

## Verify

- `go test ./internal/quota/...` (unit tests: limit resolution, override vs.
  default, `enforce` flag, `WrapRegister`'s blueprint-exists branch, 402 body
  wording, spend-ceiling resolution).
- `go test ./handlers/... -run QuotaIncrease` (request/list handler tests).
- `go test ./internal/admingrpc/... -run QuotaIncrease` (approve/deny
  transaction behavior, including the non-managed-resource rejection).
- `go test ./handlers/... -run SetThresholds` (the per-account ceiling bounds
  the spend-threshold write).
- `go test ./internal/riverqueue/... -run GatewayBudget` (a granted ceiling
  raises the gateway clamp).
- `DATABASE_URL=... go test -tags integration ./internal/quota/... ./internal/admingrpc/... -run "AgainstPostgres|Coexists|SpendLimitRequest"`
  (the spend-limit key on real Postgres, and the full request → approve →
  raised-ceiling chain).
- `cd apps/astro-client && bun x vitest run src/components/settings/ResourceLimitsSection.test.tsx src/components/RequestIncreaseDialog.test.tsx src/components/settings/ManageLimitsDialog.test.tsx`
- `Settings/ManageLimitsDialog` in Storybook (`bun x storybook dev`) renders
  the ceiling block, the handoff, and a granted ceiling without a backend.
