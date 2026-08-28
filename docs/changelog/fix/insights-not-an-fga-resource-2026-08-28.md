# Insights is an account permission, not an FGA resource

## Summary

The private-by-default authorization model registered `insights` as a WorkOS resource type parented to the account, with one singleton instance created alongside every organization account. That was a modelling mistake. Nothing creates Insights, there is never more than one per account, and no product surface grants access to it, so the resource existed only to hold two permissions. Insights is now a pair of permissions on the `account` resource type, alongside billing, audit log, and data sources.

## Design

An FGA resource type earns its place when instances are created, named, deleted, and assigned roles independently. Insights fails every part of that test: it is a read-only view of the account's own telemetry, and the only subject that could ever hold a role on it is the account itself. Registering a singleton child per account bought a resource whose lifecycle and assignment set were both fixed, and it cost a WorkOS write on every account create plus a row in the backfill and Queen inventory.

The two meaningful permissions move to `account`:

| Permission | Boundary |
| --- | --- |
| `insights:read_summary` | The account aggregate plus the caller's own rows. |
| `insights:read_members` | The per-developer breakdown. |

The pair splits on what the caller sees, not on how much of one thing they read, so the slugs name the two views rather than grading a single `read`. `insights:manage_access` is gone. It only made sense as "grant a role on the Insights resource", and there is no such resource.

Role placement follows the permission's subject rather than the resource ladder. Every member holds `insights:read_summary`, which matches the shipped behavior where aggregate spend is visible to the whole account. `insights:read_members` sits with `audit_log:read` and `data_source:manage` on maintainer and admin: whoever governs the ingest keys and reads the audit log also reads the per-developer breakdown those keys produce. Withholding it from the role that manages the data sources would have been incoherent.

The `insights-viewer` and `insights-admin` resource roles are deleted with the type. The model now has 49 permissions across 6 resource types and 22 roles (`jq '.permissions | length' scripts/workos-fga/model.json`).

Account creation registers the account and nothing else. The registration helper is singular because the account is the only resource it ever created beneath the WorkOS-required root; every other resource registers from its own create path.

Enforcement is unaffected. Insights visibility still runs on the pre-FGA `org:manage` check in `handlers/insights_visibility.go`, which the FGA rollout replaces in a later phase. This change fixes the shape of the model that phase will enforce.

## Migration

No product or user action. Staging WorkOS holds no `insights` permissions, roles, or resource instances, so the operator work is:

1. Create `insights:read_summary` and `insights:read_members` on the `account` resource type (`scripts/workos-fga/apply-permissions.sh --apply`).
2. Set the account role permission lists (`scripts/workos-fga/apply-roles.sh --apply`).
3. Delete the `insights` resource type in the WorkOS dashboard. Resource types have no public API, and this one holds nothing.

Production has no FGA model configured yet, so it takes the corrected model on first setup.
