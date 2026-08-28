# Derive the roles fine-grained access cannot ask a user for

## Summary

Under private-by-default access, a resource is reachable only through a role.
Two roles were missing, and both would have taken access away from people who
already had it the moment enforcement turned on.

The first is the creator's. Registering a deployment or blueprint in WorkOS gave
it an identity but no assignment, so a new resource reached only the account
admins, including for the person who had just created it. The lifecycle rewrite
that replaced the deployment sync ledger with direct registration dropped the
creator assignment along with it; the `creator_assignment_pending` column in the
now-dead `deployment_fga_sync` table is what remains of the old one.

The second is each member's account role. Nothing wrote one outside the Queen
backfill, which covers the account owner only. Every other member would have
arrived at the cutover holding no account role at all.

Neither gap is visible today, because enforcement is off in every environment.
Both would have been visible as a mass loss of access on the first account
enabled.

## Design

Astro derives both roles rather than asking for them, and records them as access
intent in `resource_access_fga_sync`, the same durable ledger the access API
writes. The existing reconciler and its one-minute sweep converge them, so a
WorkOS failure retries instead of losing the grant. `authz.RoleProjector` owns
the derivation; a nil projector is a no-op, which is how a deployment without
WorkOS configured behaves.

**Creator admin.** Deployment create, blueprint create, blueprint register, and
blueprint transfer each record `<type>-admin` for the caller. The intent is
recorded even when the WorkOS registration call just failed: the backfill
creates the resource later and the ledger is still pointing at the right
assignment. A machine caller has no membership to assign, so it is skipped.

**Account role.** A member's WorkOS organization role projects onto the Account
resource: owner and admin become `account-admin`, every other role becomes
`account-member`. `account-maintainer` is never derived, so a maintainer stays
someone a person chose.

Five paths write it, because no single one sees every member. Member add, role
change, and removal cover direct management. Organization provisioning covers
the account owner, who is never added through a member-add call, and repeats on
the hourly sweep. Login-time reconciliation covers everyone else, including the
case that matters most: an invited member's membership first appears at login.

Derivation deliberately ignores the rollout gates. The ledger and WorkOS
converge while enforcement is off, so enabling an organization's experiment
changes what is enforced, not who has access.

Two supporting changes make that safe:

- The role catalog registers the account and blueprint role ladders, not just
  the deployment one. Reconciliation removes a stale direct role only for a
  registered slug, so without this a demotion from `account-admin` to
  `account-member` would add the new role and leave the old one in place. The
  entries carry no actions yet, because assignment needs the slug and only
  checking needs the bundle.
- `ResourceDeleted` answers for blueprints and accounts, not deployments alone.
  It is how the reconciler tells a deleted resource from a WorkOS error, and
  without it a role intent for a deleted blueprint would retry forever.

## Migration

None. No schema change, no configuration change, no behavior change while
`FGA_ENFORCEMENT_ENABLED` is off. Existing members and existing resources still
need the backfill pass tracked in
[the migration checklist](../../06-plan/fgac-migration-tracking.md); this change
only fixes the ongoing write path, so that backfill runs once instead of falling
behind as soon as it finishes.
