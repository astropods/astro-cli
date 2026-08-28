# Fine-grained access migration checklist

**Status:** In progress
**Last updated:** 2026-08-28

Cutover from organization-membership authorization to WorkOS FGA. The design is
[Private-by-default fine-grained access rollout](../01-spec/private-by-default-fgac-rollout.md);
the WorkOS values are [WorkOS FGA setup](../05-implementation/workos-fga-setup.md).
Tick items here as they land. Delete this file once phase 6 is done, folding
anything still true into [Fine-grained access control](../03-architecture/fine-grained-access-control.md).

Verify the area after any change:

```
cd apps/astro-server && go test ./internal/org/... ./internal/authz/... ./internal/authorizationstore/... ./internal/authorizationadmin/... ./internal/authzbackfill/...
```

Legacy is still the only enforced system: `RequireAccountPermission` on 9 route
groups, `RequireAccountMember` on 7, `RequireAccountOwner` on 1, all in
`apps/astro-server/main.go`. FGA enforcement exists for `/deployments/:id` only
and is off everywhere. Preview runs shadow; production runs neither
(`config/astro-server/preview.env`, `config/astro-server/prod.env`).

## Phase 0: shipped

- [x] WorkOS model configured: 46 permissions, 19 roles (`scripts/workos-fga/model.json`, applied by `apply-permissions.sh` and `apply-roles.sh`; resource types by hand in the dashboard)
- [x] Account-rooted resource tree registered and deleted directly on create and delete (`handlers/authorization_resources.go`, called from `accounts.go`, `deploy.go`, `agents.go`, `transfer.go`, `knowledge.go`, `internal/accountlifecycle/delete.go`, `internal/riverqueue/undeploy.go`)
- [x] Historical resource backfill, run from Queen (`internal/authzbackfill/`, `internal/authorizationadmin/`)
- [x] Deployment checks, discovery, capabilities, access API, and groups (`internal/authz/`, `handlers/deployment_access*.go`)
- [x] Environment kill switch and per-account experiment (`internal/config/config.go:463`, `internal/experiment/store.go:14`)

## Phase 1: assignment data

Nothing else matters until a real user keeps their access across the flip.

- [x] Grant `<type>-admin` to the creator when a deployment or blueprint is created, recorded through `resource_access_fga_sync` so a WorkOS failure retries (`internal/authz/role_projection.go`)
- [x] Assign and update `account-admin` and `account-member` from `org.Sync.AddMember`, `ChangeMemberRole`, `RemoveMember`, organization provisioning (the owner), and login-time reconciliation (invited members). `account-maintainer` stays deliberate, never derived
- [ ] Backfill account roles for existing members, and `<type>-admin` for resolvable creators (spec PR7). The current backfill assigns the account owner only
- [ ] Backfill runs clean in Preview: zero missing `account_member_workos` rows, zero unresolved creators, or an accepted list of both

## Phase 2: blueprint identity

- [ ] `agents.uid` backfilled to zero nulls in Preview and production (`sql/astro-server/schema.sql:351`)
- [ ] Add the `agents.uid` default, `NOT NULL`, and unique constraint

## Phase 3: extend the model past deployments

- [ ] Add `account` and `blueprint` actions (`internal/authz/actions.go` holds deployment slugs only)
- [ ] Fill in the action bundles for the account and blueprint roles. `internal/authz/access_catalog.go` registers their slugs already, with empty actions, because assignment needs the slug and checking needs the actions
- [ ] Add account and organization resolvers per resource type (`deployment_account_resolver.go:45` rejects every non-deployment type)
- [ ] Catalog and classify the account and blueprint routes, with the same startup validation the deployment catalog has
- [ ] Add readable-resource discovery for blueprint lists

## Phase 4: shadow comparison

- [ ] Record `legacy_allowed`, `fga_allowed`, resource, action, route, account, and reason instead of only logging (`internal/middleware/deployment_authz.go`)
- [ ] Cover account and blueprint routes, not just deployments
- [ ] Show the comparison in Queen
- [ ] Run shadow in production with enforcement off, and wait for a clean window. The signal is `fga_allowed=false` where legacy allowed: that is a missing assignment, not a policy difference

## Phase 5: enable per account

Preview first, then production in cohorts. Per account, confirm before flipping
`fine_grained_access`, on top of the spec's own
[enforcement gates](../01-spec/private-by-default-fgac-rollout.md#enforcement-gates):

- [ ] Every eligible resource is registered with the expected Account parent. The gate no longer excludes unregistered resources, so an unregistered one fails closed (`deployment_account_resolver.go:36` returns true for any organization account)
- [ ] Every current member has a WorkOS membership ID. A missing mirror returns 503 (`internal/authz/fga_checker.go:11`, surfaced at `internal/middleware/deployment_authz.go:157`)
- [ ] Every current member has an account role, and every resolvable creator holds `<type>-admin`
- [ ] Shadow shows no unexplained grant or denial for the account
- [ ] The kill switch was exercised in the same environment

Rollback stays data-preserving: disable the account's experiment, or set
`FGA_ENFORCEMENT_ENABLED=false` for the whole environment. Keep every WorkOS
resource, assignment, and intent ledger row, and leave shadow on to compare the
recovered legacy behavior.

## Phase 6: remove legacy

- [ ] Map stored machine-app scopes to the new slugs. `RequireAccountPermission` matches `app.Scopes` against the legacy slugs, while `handlers/apps.go:101` already lists WorkOS permissions
- [ ] Remove `RequireAccountPermission`, `RequireAccountMember`, and `RequireAccountOwner` wiring from `apps/astro-server/main.go`
- [ ] Remove the legacy JWT permission slugs from the WorkOS organization roles
- [ ] Drop `deployment_fga_sync` (`sql/astro-server/schema.sql:452`), already unreferenced in Go
- [ ] Rewrite [Fine-grained access control](../03-architecture/fine-grained-access-control.md) and delete this file

## Drift to fix in the as-built doc

`03-architecture/fine-grained-access-control.md` is stamped authoritative and
last verified 2026-08-26. Three claims still disagree with the code. Fix them
with the phase 6 rewrite, or sooner if someone leans on one of them.

- [ ] "Deployments sit directly beneath their organization": they are parented under `account` (`handlers/authorization_resources.go:32`)
- [ ] "The deployment has entered the FGA lifecycle": that ledger is gone from Go, replaced by `DeploymentAccountResolver.Enabled`
- [ ] "Resource not yet managed: legacy behavior": every organization deployment is in scope once the flags are on (phase 5)
