# Vault authorization moves to existing org role permissions

## Summary

The vault REST routes checked `variable:read` and `variable:write`. Neither
slug exists as an org role permission in WorkOS any more. The FGA model
(`scripts/workos-fga/model.json`) claims `variable:read` for the `variable`
resource type, and WorkOS permission slugs are global, so the org role
version of the slug is gone. `variable:write` was dropped without a
replacement; the FGA vocabulary spells the write permission `variable:edit`.

The result was a vault no one could reach. No JWT carries either slug, so
`RequireAccountPermission` denied every request with a 403, for owners and
admins alike.

The routes now check permissions that org roles still carry:
`deployments:read` for reads and `org:manage` for writes. This is a
stopgap until the vault has its own slugs again.

## Design

Both route groups in `setupRoutes` keep their existing shape. Only the
permission argument changes.

| Routes | Old | New | Roles that pass |
|---|---|---|---|
| `GET .../variables`, `GET .../variables/:varName` | `variable:read` | `deployments:read` | owner, admin, member |
| `POST`, `PUT`, `DELETE` on the same paths | `variable:write` | `org:manage` | owner, admin |

Writes land where they were. `org:manage` is an owner-and-admin permission,
which is exactly who held `variable:write`.

Reads widen. Every org role carries `deployments:read`, so members can now
list vault entries and read the plaintext value of non-secret variables.
Secret values stay unreachable: `GetAccountVariable` only sets `Value` when
`v.Secret` is false, and no route decrypts a secret for a reader.

That widening is a deliberate trade rather than an accident of slug choice.
The deploy form's `VaultPicker` lists vault entries so a deployer can bind a
variable to a field, and members can deploy. Gating reads on `org:manage`
too would keep the old boundary but break the picker for members. Reads ride
on the same permission as viewing a deployment, which is the closest existing
match to what the picker needs.

The client needed no gate change. `VaultPicker`'s `canCreate` mirrors the
write gate by role (`admin` or `owner`), not by slug, and those are the roles
`org:manage` selects.

Machine apps authorize through the same string. `appHoldsScope` matches the
permission against the app's own scope list, so an app that reaches the vault
needs `deployments:read` or `org:manage` in its scopes. App scopes come from
WorkOS's live permission list, so no existing app can hold the removed slugs.

## Migration

No action required in the WorkOS dashboard. The change exists specifically so
that no new permission slug has to be defined.

Remove `variable:read` and `variable:write` from any org role that still lists
them. They resolve to nothing and the vault no longer reads them.

Org members gain read access to org vault variable names, descriptions, and
non-secret values. Move any non-secret variable whose value should stay with
owners and admins to a secret, or hold the value outside the vault, before
deploying.
