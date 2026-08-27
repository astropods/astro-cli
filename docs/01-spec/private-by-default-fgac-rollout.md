# Private-by-default fine-grained access rollout

**Status:** Proposed

**Updated:** 2026-08-26

## Goal

Make Astro accounts the root of product authorization. Private Blueprints and deployments are visible only through an account-level or resource-level role.

- `account-admin` sees and manages every resource in the account.
- `account-maintainer` runs the account but inherits nothing from its resources.
- `account-member` has no Blueprint or deployment permissions.
- Resource roles are a four-rung ladder: Viewer, Writer, Maintainer, Admin.
- Viewer reads. Writer changes content. Maintainer also operates the resource. Admin also deletes it and grants access.
- Personal accounts keep their single-owner behavior.

WorkOS forces every FGA tree to have an Organization root. Astro uses it only as a vendor tenancy envelope and membership identity. It is not shown in Queen or product access UI, and final authorization never depends on permissions assigned at that root.

```text
WorkOS-required root
└── Account
    ├── Blueprint
    ├── Deployment
    ├── Variable
    ├── Audience
    ├── Insights
    └── Knowledge store
```

V1 covers Account, Blueprint, and Deployment. Public Blueprint discovery remains a separate public surface. Variables, audiences, Insights, and knowledge stores come later through the same Account parent.

## Resource inventory

Types are singular snake_case, permissions are `<type>:<action>`, roles are `<type>-viewer`, `<type>-writer`, `<type>-maintainer`, `<type>-admin`. Account permissions keep their subject's namespace: `variable:read`, not `account:read_variables`.

Every slug is also a machine-app scope, because `app-scopes` returns the WorkOS permission list verbatim.

### Resource types

Roles are in [Role contract](#role-contract).

| Group | WorkOS resource type | Parent | Phase |
| --- | --- | --- | --- |
| Account | `account` | WorkOS-required root | V1 |
| Blueprint | `blueprint` | `account` | V1 |
| Deployment | `deployment` | `account` | V1 |
| Variable | `variable` | `account` | Later |
| Audience | `audience` | `account` | Later |
| Insights | `insights` | `account` | Later |
| Knowledge store | `knowledge_store` | `account` | Later |

Sub-surfaces stay with their parent and get no type of their own: datasets, evaluations, files, logs, traces, chat history, watchers, and ingestion under Deployment; versions, builds, GitHub links, avatars, and README assets under Blueprint.

| Type | Notes |
| --- | --- |
| `variable` | The account vault, `account_variables`. A deployment's own env is `deployment_build_env`, covered by `deployment:read` and `deployment:edit`. A secret's value is never returned by any API. |
| `audience` | Defined in [Access audiences](access-audiences-spec.md). `audience:manage_members` is separate from `audience:edit` so a governance connector cannot rename. Membership still grants only agent access, never a platform permission. |
| `insights` | One per account, read-only, so no create permission. `insights:read` is the aggregate plus the caller's own rows; `insights:read_members` is the per-developer breakdown. The per-source breakdown is a view of the telemetry, so it needs no permission of its own; `data_source:*` governs the ingest keys that produce it. |

Creating a child resource is an Account permission. The creator receives `<type>-admin`.

| Child type | Create permission |
| --- | --- |
| Blueprint | `blueprint:create` |
| Deployment | `deployment:create` |
| Variable | `variable:create` |
| Audience | `audience:create` |
| Knowledge store | `knowledge_store:create` |

### Account permissions

Each slug is a permission on the `account` resource type. [Role contract](#role-contract) says which role holds which.

| Group | Astro surface | Permissions |
| --- | --- | --- |
| Account record | Name, avatar, profile, experiments | `account:read`, `account:edit` |
| Account deletion | Delete the account | `account:delete` |
| Members | Members, invitations, roles | `member:read`, `member:manage` |
| Groups | Groups and their membership | `group:read`, `group:manage` |
| Machine apps | Apps and their secrets | `app:read`, `app:manage` |
| Data sources | Ingest keys for external tools, and their exclusions | `data_source:read`, `data_source:manage` |
| Billing | Usage, invoices, balances, thresholds, payment methods, quota requests | `billing:read`, `billing:manage` |
| Audit log | Account audit log | `audit_log:read` |
| Integrations | GitHub, Slack, and Supabase connections | `integration:read`, `integration:manage` |
| Clusters | Clusters assigned to the account | `cluster:read` |

Notification preferences are per user and need no permission.

### Outside the tree

| Group | Reason |
| --- | --- |
| Agent invocation | `deployment_authorization_grants`, data plane, written only by the deploy spec. The Audience it names is in the tree. |
| Queen administration | Staff surface above every tenant. |
| Public Blueprint catalog | Public, no permission. |

## Role contract

Every resource type uses the same four-rung ladder. Each rung adds to the one before it, so `maintainer` holds everything `writer` holds. A type skips a rung when it has no action at that level.

| Type | `-viewer` | `-writer` | `-maintainer` | `-admin` |
| --- | --- | --- | --- | --- |
| `deployment` | `read` | `edit` | `operate` | `delete`, `manage_access` |
| `blueprint` | `read` | `edit` | `operate` | `delete`, `manage_access`, `transfer` |
| `variable` | `read` | `edit` | skipped | `delete`, `manage_access` |
| `audience` | `read` | `edit` | `manage_members` | `delete`, `manage_access` |
| `knowledge_store` | `read` | `edit` | `operate` | `delete`, `manage_access` |
| `insights` | `read` | skipped | skipped | `read_members`, `manage_access` |

`operate` acts on the running thing without changing it: for a deployment, redeploy, roll back, restart, stop, resume, cancel, and trigger ingestion; for a blueprint, trigger a rebuild and manage the GitHub link. `manage_access` covers role grants and public visibility, since making a blueprint public is an access decision.

Account has three roles instead of the ladder. Only Admin inherits child permissions, so a team lead runs the account without gaining access to the work inside it.

| Role | Adds | Inherits child permissions |
| --- | --- | --- |
| `account-member` | `account:read`, `member:read`, `group:read`, `cluster:read` | No |
| `account-maintainer` | `account:edit`, `member:manage`, `group:manage`, `app:read`, `app:manage`, `data_source:read`, `data_source:manage`, `integration:read`, `integration:manage`, `audit_log:read`, `billing:read` | No |
| `account-admin` | `account:delete`, `billing:manage` | Yes, all |

Admin is the only role that can delete the account or change the payment method, and the only one with recovery access to a resource whose admins have all left. A finance-only role that holds `billing:manage` and nothing else comes later if needed.

These slugs are external contracts with WorkOS. Only `account-admin` inherits into child resources. Resource roles affect one resource only.

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

### `groups`

The object is a group, not an access group. WorkOS calls it a group, there is no directory sync for the shorter name to collide with, and no client UI ships it yet. PR5 renames the `access_groups` table, `handlers/access_groups.go`, and the `/access-groups` routes.

The WorkOS group APIs already exist in `internal/authz/workos_groups.go`. WorkOS remains authoritative for name, description, membership, and role assignments.

Astro adds only the metadata it owns:

- `id uuid` as the Astro group ID.
- `account_id uuid` as the local foreign key.
- `workos_group_id text UNIQUE` as the WorkOS mapping.
- `created_by text`, `created_at`, and `updated_at`.

Astro APIs use `groups.id` and resolve it to `workos_group_id`. Existing `resource_access_fga_sync` remains the durable role-assignment ledger.

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
- Keep the destructive action behind `FGA_AUTHORIZATION_RESET_ENABLED`; validate it in Preview before a manual production release.

**Queen proof:** the inventory matches WorkOS, the dry run shows exact targets, and the operation reports every deletion or failure.

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
- Add `groups` metadata and map every Astro group to its WorkOS group, renaming the `access_group` table, handler, routes, and slugs.
- Add the Organization Settings UI for create, rename, delete, add member, and remove member.
- Create and mutate the actual group in WorkOS through Astro APIs.
- Assign Viewer, Writer, Maintainer, or Admin to a group through `resource_access_fga_sync`.
- Add Queen group detail with members, resource assignments, and sync errors.

**Queen proof:** every Astro group maps to one WorkOS group; membership and assigned resources match; group-derived access is visible.

### PR6: Reusable role-assignment flyout

- Build one flyout component for Blueprint and Deployment.
- Render it only for effective `blueprint-admin` or `deployment-admin`.
- List people and groups with their strongest effective role and access source.
- Assign Admin, Maintainer, Writer, Viewer, or no direct role through Astro APIs only.
- Save explicitly and show pending or failed assignment state.

**Queen proof:** every flyout change appears with the same subject, role, source, and sync state in Queen.

### PR7: Backfill remaining account and creator roles

- Map current WorkOS owners and admins to `account-admin` on the Account. Backfilling admins as `account-maintainer` instead would revoke their access to existing resources, so narrowing them is a separate decision after enforcement is stable.
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

**Queen proof:** Account admins see all children; resource admins, maintainers, writers, viewers, and groups see only assigned resources; unassigned members see none; Queen and product/API results match.

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
