# Private-by-default fine-grained access rollout

**Status:** Proposed

**Updated:** 2026-08-25

## Goal

Make Astro accounts the root of product authorization. Private Blueprints and deployments are visible only through an account-level or resource-level role.

- `account-admin` sees and manages every resource in the account.
- `account-member` has no Blueprint or deployment permissions.
- Resource Admin can view, edit, delete, and manage the resource.
- Resource Editor can perform the resource's normal editing work but cannot delete it or manage access.
- Resource Viewer can only view.
- Personal accounts keep their single-owner behavior.

WorkOS forces every FGA tree to have an Organization root. Astro uses it only as a vendor tenancy envelope and membership identity. It is not shown in Queen or product access UI, and final authorization never depends on permissions assigned at that root.

```text
WorkOS-required root
└── Account
    ├── Blueprint
    └── Deployment
```

V1 covers Account, Blueprint, and Deployment. Public Blueprint discovery remains a separate public surface. Datasets and knowledge stores come later through the same Account parent.

## Role contract

| Resource | Role | Access |
| --- | --- | --- |
| Account | `account-member` | No child-resource permissions. |
| Account | `account-admin` | All Account, Blueprint, and Deployment permissions. |
| Deployment | `deployment-viewer` | `deployment:read` |
| Deployment | `deployment-editor` | `deployment:read`, `deployment:edit`, `deployment:operate` |
| Deployment | `deployment-admin` | `deployment:read`, `deployment:edit`, `deployment:operate`, `deployment:delete`, `deployment:manage_access` |
| Blueprint | `blueprint-viewer` | `blueprint:read` |
| Blueprint | `blueprint-editor` | `blueprint:read`, `blueprint:edit` |
| Blueprint | `blueprint-admin` | `blueprint:read`, `blueprint:edit`, `blueprint:delete`, `blueprint:manage` |

These slugs are external contracts with WorkOS. Account roles inherit into child resources. Resource roles affect one resource only.

## IDs and backfill sources

Astro IDs are the canonical WorkOS `external_id`. WorkOS `authz_resource_*` IDs are stored only for Queen, support, and reconciliation.

| Resource | Astro row | WorkOS external ID | Name | Admin source | Included in backfill |
| --- | --- | --- | --- | --- | --- |
| Account | `accounts` | `accounts.id` | `display_name`, then `name` | `accounts.owner_user_id` | `type = 'organization' AND deleted_at IS NULL` |
| Blueprint | `agents` | New immutable `agents.uid` | `agents.name` | New `agents.created_by`; historical `agent.register` audit actor | `archived_at IS NULL` in an active account |
| Deployment | `deployments` | `deployments.id` | `display_name`, then `agent_name` | `deployments.deployed_by` | `status <> 'undeployed'` in an active account |

`accounts.owner_user_id` is already `NOT NULL`, indexed, and constrained to an `account_members` row in the same account. PR4 can safely resolve it through `account_member_workos` and assign `account-admin`.

PR3 adds:

- Add `agents.uid uuid` as nullable. Adding it with volatile `DEFAULT gen_random_uuid()` would rewrite the existing table while holding an `ACCESS EXCLUSIVE` lock; PostgreSQL's fast add-column path applies only to constant defaults. Every new Blueprint write supplies a UUID immediately, and PR4 fills historical rows before adding the database default and `NOT NULL`.
- `agents.created_by text` records the creating WorkOS user so Astro can resolve their Account membership and assign `blueprint-admin`.

The existing `(account_id, name)` primary key stays in place, and `agent_versions` keeps its current `ON UPDATE CASCADE` foreign key to that key. `agents.uid` is the immutable WorkOS identity, not a replacement domain key in this rollout.

## Local storage

### `authorization_resource_sync`

One generic resource lifecycle table replaces `deployment_fga_sync`.

- Identity: `account_id`, `organization_id`, `resource_type`, `resource_id`, `workos_authorization_resource_id`.
- Hierarchy: `parent_resource_type`, `parent_resource_id`.
- Desired state: `desired_name`, `desired_state`, `desired_version`.
- Applied state: `synced_state`, `synced_version`, `synced_at`.
- Repair: `attempt_count`, `last_error`, `next_attempt_at`, `updated_at`.
- Primary key: `(organization_id, resource_type, resource_id)`.

As in `resource_access_fga_sync`, `organization_id` is the WorkOS organization ID. `resource_id` and `parent_resource_id` are Astro external resource IDs.

River applies the newest desired version after the Astro transaction commits. A WorkOS failure records retry state and never rolls back the Astro resource.

### `access_groups`

The WorkOS group APIs already exist in `internal/authz/workos_groups.go` and `handlers/access_groups.go`. WorkOS remains authoritative for name, description, membership, and role assignments.

Astro adds only the metadata it owns:

- `id uuid` as the Astro group ID.
- `account_id uuid` as the local foreign key.
- `workos_group_id text UNIQUE` as the WorkOS mapping.
- `created_by text`, `created_at`, and `updated_at`.

Astro APIs use `access_groups.id` and resolve it to `workos_group_id`. Existing `resource_access_fga_sync` remains the durable role-assignment ledger.

## Queen contract

Queen is the proof surface for every phase.

The resource table shows only:

- Type and name.
- Astro external ID and WorkOS resource ID.
- Account.
- Direct admins and assignment count.
- Creation time, sync state, and last error when present.

Filters are account, resource type, sync state, and has-error, plus search by name or either ID.

Queen also provides separate views for groups, assignments, shadow comparisons, and administrative operations.

## Rollout plan

### PR1: Queen resource control plane

- Add the Queen Authorization area and AdminService RPCs.
- Implement the resource table and filters above.
- Add **Nuke all resources** as a server-side job with dry run, typed count confirmation, progress, audit, and retry.
- Add `authorization_admin_operations` for operation state and counts.
- Add maintenance mode that pauses lifecycle writes, pauses the River sweep, and drains running FGA jobs before deletion.
- Keep the destructive action disabled by default and Preview-only first.

**Queen proof:** the inventory matches WorkOS, the dry run shows exact targets, and deletion cannot start while reconciliation is active.

### Phase 2: Preview deployment reset

- This is an audited Preview operation through PR1, not a code PR.
- Enter maintenance mode and drain FGA jobs.
- Remove each deployment's role assignments, then delete every current WorkOS Deployment resource without cascade.
- After Queen shows zero Deployments, change the WorkOS resource model so Blueprint and Deployment use Account as their parent.
- Keep the WorkOS-required root intact.
- Save the operation report and leave maintenance mode on until PR3 deploys.

**Queen proof:** Preview shows zero Deployment resources, zero failed deletions, and no resources being recreated.

### PR3: Register new resources

- Configure Account, Blueprint, and Deployment resource types and final roles in WorkOS.
- Add nullable `agents.uid` and make every new Blueprint insert supply its stable WorkOS external ID. Record its creator in `agents.created_by` for future `blueprint-admin` assignment.
- Add `authorization_resource_sync` to record which Account, Blueprint, and Deployment resources Astro expects in WorkOS, which version WorkOS has confirmed, and any retry error. River processes it and Queen displays it.
- Add one generic River resource reconciler.
- Register the Account first under the WorkOS-required root.
- Register every new Blueprint and Deployment with that Account as its WorkOS parent.
- Synchronize names and deletions and store the returned WorkOS resource ID.
- Stop creating new `deployment_fga_sync` intent. Do not enforce access yet.
- Leave maintenance mode only after Queen verifies the first complete Account hierarchy.

**Queen proof:** a newly created Account, Blueprint, and Deployment appear with the expected Astro ID, WorkOS ID, Account, name, and synced state.

### PR4: Backfill existing resources and account owners

- Fill null `agents.uid` values in bounded batches ordered by the existing `(account_id, name)` key.
- Build the unique `agents.uid` index concurrently. Add a `uid IS NOT NULL` check as `NOT VALID`, validate it separately, then set `DEFAULT gen_random_uuid()` and `NOT NULL` without rewriting existing rows; remove the temporary check afterward.
- Queue active Accounts, then active Blueprints and Deployments, using the exact source columns above.
- Register in bounded, restartable batches and populate `workos_authorization_resource_id`.
- Resolve `accounts.owner_user_id` through `account_member_workos` and assign `account-admin` to each Account owner.
- Report missing WorkOS memberships and failed resources without enabling enforcement.
- Remove `deployment_fga_sync` only after every live Deployment is represented by the generic ledger and old jobs are empty.

**Queen proof:** Preview shows every eligible Account, Blueprint, and Deployment; counts match Astro; every row is synced to the correct Account; and every Account shows its owner as a direct admin.

### PR5: Group metadata and UI

- Reuse the existing WorkOS group SDK and Astro group APIs.
- Add `access_groups` metadata and map every Astro group to its WorkOS group.
- Add the Organization Settings UI for create, rename, delete, add member, and remove member.
- Create and mutate the actual group in WorkOS through Astro APIs.
- Assign Viewer, Editor, or Admin to a group through `resource_access_fga_sync`.
- Add Queen group detail with members, resource assignments, and sync errors.

**Queen proof:** every Astro group maps to one WorkOS group; membership and assigned resources match; group-derived access is visible.

### PR6: Reusable role-assignment flyout

- Build one flyout component for Blueprint and Deployment.
- Render it only for effective `blueprint-admin` or `deployment-admin`.
- List people and groups with their strongest effective role and access source.
- Assign Admin, Editor, Viewer, or no direct role through Astro APIs only.
- Save explicitly and show pending or failed assignment state.

**Queen proof:** every flyout change appears with the same subject, role, source, and sync state in Queen.

### PR7: Backfill remaining account and creator roles

- Map current WorkOS owners/admins to `account-admin` on the Account.
- Map ordinary members to `account-member`, which has no child permissions.
- Assign `deployment-admin` from `deployments.deployed_by` when the membership still exists.
- Assign `blueprint-admin` from `agents.created_by` or the earliest trustworthy `agent.register` audit actor.
- Leave unresolved creators unassigned; Account admins retain recovery access.
- Run through versioned access intent with dry run, retry, and counts.

**Queen proof:** every Account member has the expected Account role, every resolvable creator is a direct resource admin, and unresolved rows are listed explicitly.

### PR8: Compare legacy and FGA decisions

- Evaluate both systems for Account, Blueprint, and Deployment lists, reads, and mutations.
- Return only the legacy result.
- Record `legacy_allowed`, `fga_allowed`, resource, action, route, account, and reason.
- Use WorkOS discovery for lists and live checks for direct actions.
- Keep shadow work off the request path and behind environment and Account flags.

**Queen proof:** the comparison view shows expected member mismatches, unexpected grants, missing identities, WorkOS errors, and latency.

### PR9: Enforce private-by-default access

- Require the global kill switch and Account rollout flag.
- Use WorkOS discovery for lists and live checks for reads and mutations.
- Remove Account, Blueprint, and Deployment permissions from WorkOS root roles after Account-role projection completes.
- Stop treating organization membership as resource access.
- Return empty private lists to unassigned Account members and concealed not-found for denied direct access.
- Preserve personal accounts and the public Blueprint catalog.
- Roll out Preview first, then production Accounts in batches.

**Queen proof:** Account admins see all children; resource admins, editors, viewers, and groups see only assigned resources; unassigned members see none; Queen and product/API results match.

### PR10: Cleanup

- Start only after PR9 is stable in Preview and the first production cohort.
- Remove legacy membership authorization and shadow-only code.
- Remove the old deployment lifecycle worker, store, role slugs, fakes, and compatibility adapters.
- Consolidate Blueprint and Deployment code behind generic resource contracts.
- Remove obsolete schema only after Queen shows no pending work.
- Keep the global kill switch until the production rollback window closes.
- Run server, client, Queen, integration, and contract suites plus an unused-code pass.

**Queen proof:** no old sync state or unexplained drift remains; production behavior is unchanged.

## Enforcement gates

PR9 cannot enable an Account until Queen confirms:

- all eligible resources are synced;
- all current members have a WorkOS membership ID and Account role;
- creator/admin backfills completed or unresolved rows were accepted;
- role configuration matches Astro constants;
- PR8 has no unexplained FGA grants;
- the kill switch was tested.
