**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

# Account lifecycle and identity

An **account** is a globally unique, human-readable namespace (`accounts.name`)
that owns agents, deployments, and every other account-scoped resource. Every
account is one of two types:

- **Personal**: one per WorkOS user, created the first time they claim a
  username. A user must have one before they can publish or deploy.
- **Organization**: a shared namespace with multiple members, roles from
  WorkOS, and its own WorkOS Organization linked 1:1 via
  `account_organizations.workos_org_id`.

Membership, roles, invitations, and fine-grained access control are covered
by the "Org / fine-grained access control" area. See
[`organizations.md`](organizations.md) and
[`fine-grained-access-control.md`](fine-grained-access-control.md). This doc
covers the account row's own lifecycle (create, rename, soft-delete, purge),
its identity surface (profile fields, avatar), and the member-email mirror.
Cluster bindings (`internal/account/clusters.go`) are covered by
[`cluster-configuration.md`](cluster-configuration.md), not here.

## Data model

`accounts` is the core row (`id`, `name`, `type`, `display_name`,
`owner_user_id`, `deleted_at`, avatar/color columns, provider customer ID
columns). Related tables, all keyed by `account_id`:

| Table | Holds |
|---|---|
| `account_members` | `(account_id, user_id)` membership pairs |
| `account_member_workos` | WorkOS membership ID per member, for role sync |
| `account_member_emails` | the member-email mirror (see below), keyed by `user_id` |
| `account_organizations` | `workos_org_id` for organization accounts |
| `account_profile` | bio, location, timezone, pronouns, website, social links, blueprint order, `account_number` |
| `account_langfuse` | Langfuse project link, read by the purger |

`internal/account` (`store.go`, `types.go`, `validate.go`, `denylist.go`,
`variants.go`) owns all of these except the purge/delete sequence itself,
which lives in `internal/accountlifecycle`.

## Account creation

`handlers.CreateAccount` (`POST /api/v1/accounts`) is the single entry point
for both account types:

1. Validate name format (`account.ValidateAccountName`: 4-39 chars, lowercase,
   starts with a letter, no leading/trailing/consecutive hyphens) and check it
   against the reserved/denied name lists (`account.CheckAccountNameRestricted`,
   several hundred reserved route/brand/product words plus a separate
   `deniedNames` set for brand-protected names).
2. `accountStore.Create` inserts the `accounts` row, the creator's
   `account_members` row, and an empty `account_profile` row in one
   transaction, then binds the account to the primary cluster
   (`account.BindPrimary`, see `cluster-configuration.md`).
3. Best-effort: mirror the creator's WorkOS email into `account_member_emails`
   (`memberemails.UpsertWorkOS`) so dev-tool telemetry attribution works
   immediately, without waiting for the reconcile job.
4. For `type: organization`, create a WorkOS Organization
   (`external_id = account.ID`), link it (`SetWorkOSOrganizationID`), and
   create the creator's WorkOS membership as `owner`. Each step has a
   compensating action on failure: a failed org link or membership create
   deletes the WorkOS org and the local account row (`DeleteByID`) rather than
   leaving an orphaned account.
5. Billing customer creation, rate-card provisioning, and the signup credit
   are enqueued off the request path (`InsertBillingProvision`). A dropped
   enqueue is picked up by an hourly sweep, not retried inline.
6. Emits an `account.create` audit log event and a best-effort welcome
   notification (`notify.AccountWelcome`).

Renaming (`RenameAccount`, `PUT /api/v1/accounts/:account`, owner only) updates
the name, syncs it to the WorkOS organization if linked, and moves the
account's and its agents' avatars in storage to the new key
(`avatar.Store.MoveAllForAccount`). See Avatars below. Account **name** is
mutable; account **ID** is the stable identity used for infrastructure
(K8s namespace, ECR paths). See `cluster-configuration.md` for namespace
derivation.

## Soft-delete → purge lifecycle

Deletion runs in two phases so it's immediate from the user's perspective but
reversible for a retention window, and so external cleanup that can fail
(WorkOS, Langfuse, AI Gateway) doesn't block the user-visible delete.

Both phases are shared code in `internal/accountlifecycle`, used identically
by the public `DELETE /api/v1/accounts/:account` handler and the admin
console (`internal/admingrpc/accounts.go`): an operator-initiated delete and
a self-service delete run the exact same sequence.

### Phase 1: `accountlifecycle.Deleter.Delete` (immediate)

Called synchronously from `handlers.DeleteAccount`. Order matters:

1. **Refuse if the account owes money.** `handlers.DeleteAccount` checks
   `outstandingBalance` (dunning status or current spend at least $0.01)
   before calling `Deleter.Delete`, because archiving the billing customer
   voids open invoices. A delete that ran first would be an unreviewed
   write-off. Returns `409` if the account owes money.
2. Archive the billing customer (`Billing.DeleteCustomer`) if one exists.
   Failure here aborts before anything is mutated, so the caller can retry.
3. `Accounts.MarkDeleted` sets `deleted_at`. This is the point of no return:
   `AccountStore.GetByName`/`GetByID`/`GetAccountsForUser` all filter
   `deleted_at IS NULL`, so a deleted account 404s immediately at
   `middleware.ResolveAccount` and disappears from the user's account list.
   Calling `MarkDeleted` twice returns `account.ErrAlreadyDeleted` (`404`).
4. Revoke the account's long-lived AI Gateway judge key
   (`RevokeAccountJudgeKeys`) now rather than at purge time. It's the one
   credential worth killing immediately rather than leaving live for the
   whole retention window.
5. Enqueue undeploy for every visible deployment (`EnqueueUndeploy`, shared
   with the public undeploy route, see `deployment-state-machine.md`).
   Failures are logged and skipped, not retried here.
6. Delete the WorkOS organization if one is linked. Best-effort.

The account row itself survives until the purge worker removes it.

### Phase 2: `accountlifecycle.Purger.Purge` (after retention)

`RetentionDays = 7`. A periodic River job (`AccountPurgeWorker`, hourly) calls
`Purger.Overdue` (accounts with `deleted_at` older than 7 days) then
`Purger.Purge` for each. The admin console can also purge one account on
demand (`PurgeAccount` gRPC).

`Purge` refuses, and is fully retryable, while teardown is outstanding:

1. **Pending deployments** (not yet `undeployed`): re-enqueues undeploy for
   any deployment that hasn't reached `undeploying`, then returns
   `accountlifecycle.ErrTeardownPending`.
2. **Pending FGA sync** (`DeploymentFGASyncStore.HasPendingForAccount`):
   returns the same sentinel if authorization tuples haven't converged.
3. **Langfuse project delete**: must succeed before the row is dropped,
   because the project holds trace data under the shared org and nothing
   else records which project to delete once the account row is gone. A
   failure here aborts the whole purge, unlike the AI Gateway steps below.
4. **AI Gateway key revocation** (regular, dev, and judge keys): best-effort.
   A gateway outage warns and continues rather than blocking the purge,
   because the account row cascade-deletes the local key rows regardless.
   LiteLLM holds no FK back to Astro, so an unrevoked key silently keeps
   working upstream with no local record it exists, a known gap this step
   does not fix.
5. `DELETE FROM accounts WHERE id = $1`, cascading every remaining child
   table.

`internal/systemaudit` runs a standing check (`account.purge_overdue`) that
flags any soft-deleted account still present more than
`RetentionDays + 1` days after `deleted_at`, with the same pending-deployment
and pending-authorization counts `Purge` itself checks. The purge sweep logs
and skips a permanently blocked account rather than erroring loudly, so this
audit check is the only thing that surfaces one.

### `05-implementation/account-system-design.md` and `06-plan/account-deletion-plan.md`

`account-deletion-plan.md` is banner-marked shipped, and the shipped code
matches its core design (synchronous soft-delete, async undeploy via River,
billing archive, audit log, retention-based purge). Two things evolved past
what the plan describes, both consistent with "shipped, not literal":

- The sequence was extracted into `internal/accountlifecycle` (`Deleter` and
  `Purger`) rather than living inline in `handlers/accounts.go`, specifically
  so the admin console's operator-initiated delete/purge and the public
  self-service path can't drift from each other.
- The shipped version adds two safeguards the plan didn't have: refusing to
  delete an account with an outstanding balance, and refusing to purge one
  with unconverged FGA sync state. Judge-key revocation was also added.

`account-system-design.md` is already correctly bannered superseded by
`organizations.md` and needs no further action from this doc.

## The member-email mirror (`internal/memberemails`)

External dev-tool telemetry (Claude Code, etc.) is stamped with the
developer's `user.email`, not their WorkOS user ID. `account_member_emails`
mirrors `user_id → email` so that telemetry can be attributed to a member with
one indexed lookup instead of a WorkOS API call per event. It also backs the
audit log's actor display and the org member list's email column when a
member has no Astro username.

**What keeps it fresh:**

- **Auth-time capture.** Login (`handlers/auth.go`) and account creation
  (`handlers/accounts.go`) both call `memberemails.UpsertWorkOS` after every
  successful WorkOS auth, so a member's mirrored email self-heals on their
  next login without needing the reconcile job.
- **Reconcile job** (`riverqueue.MemberEmailReconcileWorker`, job kind
  `workos.member_email_reconcile`). Finds members with no recorded email
  (`UserIDsMissingEmail`, capped at 500/run, 8-way concurrent WorkOS lookups)
  and resolves them via `WorkOSClient.GetUser`. A user who resolves to
  "no email" or a definitive WorkOS 404 is recorded with
  `RecordReconcileAttempt` and left alone for `RetryBackoff` (6 hours). This
  backfills pre-existing members and heals gaps, since auth-time capture
  only covers logins going forward.
- **Deletion.** `DeleteForUser` removes a user's mirrored email and backoff
  record on WorkOS user deletion.

Callers reading the mirror (`org.go`'s `resolveMemberEmails`,
`AccountStore.GetOwnerEmail`) prefer the local mirror and fall back to a
direct WorkOS lookup, respecting the same `RetryBackoff` so an unnameable
member isn't re-queried on every page view.

## Avatars and identity images

`internal/avatar.Store` is a thin key/value layer over a pluggable `Backend`
(S3 in production, local disk in dev) for three avatar kinds, all JPEG at the
storage layer regardless of upload format: account avatars
(`avatars/{handle}.jpg`), blueprint avatars
(`avatars/agents/{account}/{name}.jpg`), and deployment avatars
(`avatars/deployments/{id}.jpg`).

**Upload pipeline** (`handlers/avatar.go`): validate size (5 MB max) and
content type, reject SVG uploads at the account/blueprint/deployment upload
endpoints (SVG is only accepted for the *preset* pipeline, not user
uploads), decode, resize to 512x512 with `draw.CatmullRom`, re-encode as
JPEG (quality 85), then write. Every write is followed by:

1. **Stamp `avatar_updated_at`** (`touchAccountAvatar`/`touchAgentAvatar`/
   `touchDeploymentAvatar`) using the value the database returns, not the app
   clock. The response URL's `?v=` cache-busting token has to match what a
   later read computes from the same column, or the same image ends up
   cached under two different URLs.
2. **Extract and store a color palette** from the freshly-written JPEG
   (`colorextract.ExtractFromJPEG`) for UI accents. Best-effort: a failure
   here is logged and the response simply omits `avatar_colors`.

Cache-Control on every avatar object is
`public, max-age=86400, stale-while-revalidate=604800`; freshness on the
common path comes from the `?v=` token changing, not the TTL.

**Presets.** 25 deterministic placeholder JPEGs
(`placeholders/accounts/avatar_NN.jpg`). `avatar.PresetIndex(handle)` picks
one by a simple string hash so a given handle always gets the same preset
until it uploads a real image.

**Procedural identity generation (`internal/identitygen`).** A Go port of a
former TypeScript package (`astro-identity-gen`, deleted after the port) that
generates a deterministic polygon-and-eyes SVG identity from a seed string,
rasterizes it to JPEG (`GenerateIdentityJPEG`), and writes it directly via
`avatar.Store.WriteAgentAvatarJPEG`, bypassing the decode/resize/re-encode
pipeline above since it already produces JPEG at the right size and quality.
This is what backs a **blueprint's** placeholder avatar when it has none: new
blueprint creation (`handlers/agents.go`) and a periodic backfill job
(`riverqueue.blueprint_avatar_backfill.go`) both call it, seeded by
account+blueprint name so the same blueprint always gets the same generated
identity. Every RNG call in `generateIdentityWithChoices` must occur in the
same sequence as the original TypeScript reference; that sequence is the
documented parity contract enforced by `identitygen`'s parity tests.

**Rename.** `avatar.Store.MoveAllForAccount` is the single entry point for
account renames and org-name-change events: it moves the account avatar and
every agent avatar under the old handle to the new one (skipping any that
don't exist), then the caller re-stamps `avatar_updated_at` on the moved keys
so their cache-busting tokens advance past the old cached copy.

## How `account`, `accountcache`, and `accountlifecycle` relate

These are a layering, not overlapping ownership of the same thing:

- **`internal/account`** is the account row's CRUD and profile store, the
  thing everything else calls into. It has no knowledge of deployments,
  billing, or WorkOS beyond the columns it stores.
- **`internal/accountlifecycle`** is a *sequence* built on top of `account`
  plus several other stores (`deploymentstore`, `billing`, `org`, `aigateway`,
  `langfuse`, `authz`). It exists because the delete/purge sequence has to
  touch all of them in a specific order and be identical whether triggered by
  a user or an operator. It has no persistence of its own.
- **`internal/accountcache`** is unrelated to either: it's a single function
  (`InvalidateAccount`) that busts the K8s-view caches (`deploycache`,
  `obssummary`) Queen and the dashboard read, for callers that changed
  something the cache doesn't know about (for example, an admin action, or
  the fine-grained-access experiment flip in `handlers/experiments.go`).
  It's a cache-invalidation helper, not an account-lifecycle store.

## Known gaps

- `internal/avatar` and `handlers/avatar.go` have no unit tests (0 `_test.go`
  files across both). The upload/resize/color-extraction/rename pipeline is
  currently verified only by exercising it through the running app.
- The AI Gateway purge step's own comment names the real limitation: LiteLLM
  holds no FK back to Astro, so a revoke call that fails during purge leaves
  a working credential upstream with nothing local to alert on it later.

## Verify

- `go test ./internal/account/... ./internal/accountcache/... ./internal/accountlifecycle/... ./internal/memberemails/... ./internal/identitygen/...`: `accountcache` has one test file exercising `InvalidateAccount`; `avatar` has none (see Known gaps).
- `go test ./handlers/... -run TestDeleteAccount` for the balance-refusal and delete-handler paths (`accounts_delete_balance_test.go`, `accounts_test.go`).
