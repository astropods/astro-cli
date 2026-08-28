# WorkOS FGA setup

Copy-paste reference for configuring WorkOS Authorization. The model is defined in [Private-by-default fine-grained access rollout](../01-spec/private-by-default-fgac-rollout.md); this page is only the values and the steps.

Configure in this order: resource types, then permissions, then roles. A role cannot reference a permission that does not exist yet, and a permission cannot be scoped to a resource type that does not exist yet.

Read [Runbook](#runbook) before applying to an environment.

## Resource types

| Slug | Parent | Description |
| --- | --- | --- |
| `account` | WorkOS-required root | An Astro account. The root of product authorization. |
| `blueprint` | `account` | A packaged agent that can be deployed. |
| `deployment` | `account` | A running agent. |
| `variable` | `account` | An account vault variable. |
| `audience` | `account` | A named list of people who may talk to an agent. |
| `knowledge_store` | `account` | A connected or managed data store an agent reads. |

## Permissions

### `account`

| Slug | Name | Description |
| --- | --- | --- |
| `account:read` | Read account | View the account's name, avatar, profile, and experiments. |
| `account:edit` | Edit account | Change the account's name, avatar, profile, and experiments. |
| `account:delete` | Delete account | Delete the account. |
| `member:read` | Read members | View members and invitations. |
| `member:manage` | Manage members | Invite members, remove them, and change their roles. |
| `group:read` | Read groups | View groups and their membership. |
| `group:manage` | Manage groups | Create, rename, and delete groups, and change their membership. |
| `app:read` | Read apps | View machine apps and their scopes. |
| `app:manage` | Manage apps | Create and delete machine apps, their scopes, and their secrets. |
| `data_source:read` | Read data sources | View data sources and their exclusion lists. |
| `data_source:manage` | Manage data sources | Create, rename, and revoke data sources, and edit their exclusion lists. |
| `insights:read_summary` | Read Insights summary | View the account's aggregate coding-tool activity and your own. |
| `insights:read_members` | Read member Insights | View each member's coding-tool activity. |
| `billing:read` | Read billing | View usage, invoices, balances, and spend thresholds. |
| `billing:manage` | Manage billing | Set spend thresholds and manage the payment method. |
| `audit_log:read` | Read audit log | View the account audit log. |
| `integration:read` | Read integrations | View GitHub, Slack, and Supabase connections. |
| `integration:manage` | Manage integrations | Connect and disconnect GitHub, Slack, and Supabase. |
| `cluster:read` | Read clusters | View the clusters assigned to the account. |
| `blueprint:create` | Create blueprints | Create blueprints in the account. |
| `deployment:create` | Create deployments | Deploy agents in the account. |
| `variable:create` | Create variables | Create account variables. |
| `audience:create` | Create audiences | Create audiences. |
| `knowledge_store:create` | Create knowledge stores | Connect or provision knowledge stores. |

### `blueprint`

| Slug | Name | Description |
| --- | --- | --- |
| `blueprint:read` | Read blueprint | View the blueprint, its versions, and its builds. |
| `blueprint:edit` | Edit blueprint | Change the blueprint's content, avatar, and README. |
| `blueprint:operate` | Operate blueprint | Trigger a rebuild and manage the blueprint's GitHub link. |
| `blueprint:delete` | Delete blueprint | Archive the blueprint. |
| `blueprint:manage_access` | Manage blueprint access | Grant and revoke access to the blueprint, and set whether it is public. |
| `blueprint:transfer` | Transfer blueprint | Move the blueprint to another account. |

### `deployment`

| Slug | Name | Description |
| --- | --- | --- |
| `deployment:read` | Read deployment | View the deployment, its configuration, logs, traces, files, and chat history. Secret values stay redacted. |
| `deployment:edit` | Edit deployment | Change the deployment's configuration and metadata. |
| `deployment:operate` | Operate deployment | Redeploy, roll back, restart, stop, resume, cancel, and trigger ingestion. |
| `deployment:delete` | Delete deployment | Undeploy the deployment. |
| `deployment:manage_access` | Manage deployment access | Grant and revoke access to the deployment. |

### `variable`

| Slug | Name | Description |
| --- | --- | --- |
| `variable:read` | Read variable | View the variable's name, description, and plaintext value. A secret's value is never returned. |
| `variable:edit` | Edit variable | Change the variable's value and description. |
| `variable:delete` | Delete variable | Delete the variable. |
| `variable:manage_access` | Manage variable access | Grant and revoke access to the variable. |

### `audience`

| Slug | Name | Description |
| --- | --- | --- |
| `audience:read` | Read audience | View the audience and its members. |
| `audience:edit` | Edit audience | Rename the audience and change its description. |
| `audience:manage_members` | Manage audience members | Add and remove audience members. |
| `audience:delete` | Delete audience | Delete the audience. |
| `audience:manage_access` | Manage audience access | Grant and revoke access to the audience. |

### `knowledge_store`

| Slug | Name | Description |
| --- | --- | --- |
| `knowledge_store:read` | Read knowledge store | View the knowledge store and its connection details. |
| `knowledge_store:edit` | Edit knowledge store | Change the knowledge store's credentials and settings. |
| `knowledge_store:operate` | Operate knowledge store | Recheck the knowledge store's connection and repair its host. |
| `knowledge_store:delete` | Delete knowledge store | Delete the knowledge store. |
| `knowledge_store:manage_access` | Manage knowledge store access | Grant and revoke access to the knowledge store. |

## Roles

Resource roles are cumulative: Maintainer holds everything Writer holds. The permission lists below are complete, so paste them as-is.

### `account`

| Slug | Name | Description |
| --- | --- | --- |
| `account-member` | Member | Belongs to the account. Sees no resource unless granted one directly. |
| `account-maintainer` | Maintainer | Runs the account without gaining access to the work inside it. |
| `account-admin` | Admin | Full control of the account and every resource in it. |

`account-member`

```
account:read
member:read
group:read
cluster:read
insights:read_summary
```

`account-maintainer`

```
account:read
account:edit
member:read
member:manage
group:read
group:manage
app:read
app:manage
data_source:read
data_source:manage
insights:read_summary
insights:read_members
integration:read
integration:manage
audit_log:read
billing:read
cluster:read
```

`account-admin`

```
account:read
account:edit
account:delete
member:read
member:manage
group:read
group:manage
app:read
app:manage
data_source:read
data_source:manage
insights:read_summary
insights:read_members
billing:read
billing:manage
audit_log:read
integration:read
integration:manage
cluster:read
blueprint:create
deployment:create
variable:create
audience:create
knowledge_store:create
blueprint:read
blueprint:edit
blueprint:operate
blueprint:delete
blueprint:manage_access
blueprint:transfer
deployment:read
deployment:edit
deployment:operate
deployment:delete
deployment:manage_access
variable:read
variable:edit
variable:delete
variable:manage_access
audience:read
audience:edit
audience:manage_members
audience:delete
audience:manage_access
knowledge_store:read
knowledge_store:edit
knowledge_store:operate
knowledge_store:delete
knowledge_store:manage_access
```

### `blueprint`

| Slug | Name | Description | Permissions |
| --- | --- | --- | --- |
| `blueprint-viewer` | Viewer | Reads the blueprint. | `blueprint:read` |
| `blueprint-writer` | Writer | Reads and changes the blueprint. | `blueprint:read`, `blueprint:edit` |
| `blueprint-maintainer` | Maintainer | Also rebuilds it and manages its GitHub link. | `blueprint:read`, `blueprint:edit`, `blueprint:operate` |
| `blueprint-admin` | Admin | Full control, including deletion, transfer, and access. | `blueprint:read`, `blueprint:edit`, `blueprint:operate`, `blueprint:delete`, `blueprint:manage_access`, `blueprint:transfer` |

### `deployment`

| Slug | Name | Description | Permissions |
| --- | --- | --- | --- |
| `deployment-viewer` | Viewer | Reads the deployment. | `deployment:read` |
| `deployment-writer` | Writer | Reads and changes the deployment. | `deployment:read`, `deployment:edit` |
| `deployment-maintainer` | Maintainer | Also redeploys, rolls back, restarts, and stops it. | `deployment:read`, `deployment:edit`, `deployment:operate` |
| `deployment-admin` | Admin | Full control, including deletion and access. | `deployment:read`, `deployment:edit`, `deployment:operate`, `deployment:delete`, `deployment:manage_access` |

### `variable`

| Slug | Name | Description | Permissions |
| --- | --- | --- | --- |
| `variable-viewer` | Viewer | Reads the variable. | `variable:read` |
| `variable-writer` | Writer | Reads and changes the variable. | `variable:read`, `variable:edit` |
| `variable-admin` | Admin | Full control, including deletion and access. | `variable:read`, `variable:edit`, `variable:delete`, `variable:manage_access` |

### `audience`

| Slug | Name | Description | Permissions |
| --- | --- | --- | --- |
| `audience-viewer` | Viewer | Reads the audience and its members. | `audience:read` |
| `audience-writer` | Writer | Also renames the audience. | `audience:read`, `audience:edit` |
| `audience-maintainer` | Maintainer | Also adds and removes members. | `audience:read`, `audience:edit`, `audience:manage_members` |
| `audience-admin` | Admin | Full control, including deletion and access. | `audience:read`, `audience:edit`, `audience:manage_members`, `audience:delete`, `audience:manage_access` |

### `knowledge_store`

| Slug | Name | Description | Permissions |
| --- | --- | --- | --- |
| `knowledge_store-viewer` | Viewer | Reads the knowledge store. | `knowledge_store:read` |
| `knowledge_store-writer` | Writer | Reads and changes the knowledge store. | `knowledge_store:read`, `knowledge_store:edit` |
| `knowledge_store-maintainer` | Maintainer | Also rechecks its connection. | `knowledge_store:read`, `knowledge_store:edit`, `knowledge_store:operate` |
| `knowledge_store-admin` | Admin | Full control, including deletion and access. | `knowledge_store:read`, `knowledge_store:edit`, `knowledge_store:operate`, `knowledge_store:delete`, `knowledge_store:manage_access` |

## Runbook

Dashboard only. The `workos` CLI manages flat permissions and organization roles, not resource types or resource-scoped roles.

Preview first, verify, then Production. Check the environment switcher before typing anything.

### Steps

1. **Authorization > Resource Types**: create each row from [Resource types](#resource-types), setting the parent.
2. Create every permission from [Permissions](#permissions), scoped to its resource type.
3. Create every role from [Roles](#roles) and attach its permission list.
4. Attach the child-type permissions to `account-admin`. A role holding child-type permissions propagates them down the hierarchy, which is how an account admin reaches every resource without a per-resource assignment.

Add before you deploy the code that checks a permission, remove only after that code is gone. A check against a missing permission fails closed.

### Verify

| Check | Expectation |
| --- | --- |
| Permission count | 49 across 6 resource types (`jq '.permissions \| length' scripts/workos-fga/model.json`) |
| Role count | 22 |
| `account-member` | No child-resource permission. This is what makes resources private by default |
| Root organization role | No product permission at all |

Then in Preview, assign `deployment-viewer` to a test membership and call `GET /api/v1/deployments/:id/capabilities`: `read` true, the other four false. Swap to `deployment-maintainer` and `operate` flips true while `delete` stays false.

### Recovery

Deleting a permission or role revokes access on save, with no propagation delay. Recreating it with the identical slug restores every assignment, because assignments are stored against the slug.

If Production is enforcing, use the kill switch rather than editing the model under load.
