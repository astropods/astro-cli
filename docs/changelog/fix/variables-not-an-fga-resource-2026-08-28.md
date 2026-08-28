# Variables are an account permission, not an FGA resource

## Summary

The private-by-default authorization model registered `variable` as a WorkOS resource type parented to the account, with one instance per row in `account_variables`. The vault is one account-scoped keyspace, not a set of independently governed things: no surface grants access to a single variable, and no role is ever assigned on one. The type is gone. Vault access is now `variable:read` and `variable:manage` on the `account` resource type, next to `app:read` and `app:manage`.

## Design

A resource type earns its place when instances are created, named, deleted, and granted roles one at a time. Variables pass the first three and fail the fourth, which is the one that matters. Per-variable roles would also be incoherent with how the vault is consumed: a deployment resolves `${variables.FOO}` at deploy time, so a member holding a role on some variables and not others would hold half a deployment's configuration.

The cost was real. Every vault write registered a WorkOS resource, every delete removed one, and the Connect WorkOS backfill enumerated `account_variables` in full. An account with 40 variables paid 40 WorkOS writes for 40 resources no one could ever be granted.

The four instance permissions collapse to the two-rung shape every other account surface uses:

| Before (`variable` type) | After (`account` type) |
| --- | --- |
| `variable:create` (already on `account`), `variable:edit`, `variable:delete` | `variable:manage` |
| `variable:read` | `variable:read` |
| `variable:manage_access` | dropped |

`variable:manage_access` only meant "grant a role on this variable", and there is no such role: `variable-viewer`, `variable-writer`, and `variable-admin` are deleted with the type. The model now has 46 permissions across 5 resource types and 19 roles (`jq '.permissions | length' scripts/workos-fga/model.json`).

Role placement preserves the shipped gate rather than narrowing it. Every member holds `variable:read`, matching vault reads riding on `deployments:read`, which every org role carries. `variable:manage` sits on maintainer with `app:manage`, matching writes riding on `org:manage`. Both surfaces hold account credentials, so they belong at the same rung. Narrowing vault reads to maintainer is a separate decision, taken after enforcement is stable.

Enforcement is unaffected. The vault still checks `deployments:read` and `org:manage` in `main.go`, because neither new slug is an org role permission and FGA does not enforce account permissions yet. `CreateAccountVariable` and `DeleteAccountVariable` no longer take a `ResourceLifecycle`, so the registration cannot come back by accident.

## Migration

No product or user action. Vault behavior, gating, and the API are unchanged.

Production has no FGA model configured, so it takes the corrected model on first setup. In staging the operator work is:

1. Create `variable:read` and `variable:manage` on the `account` resource type (`scripts/workos-fga/apply-permissions.sh --apply`).
2. Set the account role permission lists (`scripts/workos-fga/apply-roles.sh --apply`).
3. Check Queen's resource inventory for `variable` rows. Any that exist were registered by a vault write and now report as `workos_only`; delete them there.
4. Delete the `variable` resource type in the WorkOS dashboard, once step 3 leaves it empty. Resource types have no public API.
