# Agent Transfers & ECR Namespace Decoupling

## Summary

Deployment artifacts are currently stored in ECR under account-name-based paths (`{env}-tenant-{account_name}/{image}`), coupling image storage to a mutable identifier. This change decouples physical image location from account ownership by adding a per-version `ecr_namespace` column, enabling zero-downtime agent transfers between accounts without image copying.

## Design

A new `ecr_namespace` column on `agent_versions` freezes the ECR namespace at push time. Deploy-time resolution (`resolveImage` in `template.go`) reads this column instead of parsing the account name from the stored image path. When empty, it falls back to the image path for backward compatibility.

**Transfer flow**: `POST /api/v1/agents/:account/:name/transfer` moves `account_id` on agent and version rows in a single transaction. Each version's `ecr_namespace` is untouched — old builds resolve to the original owner's ECR repos, new builds pushed under the new owner get the new namespace. No image copying, no redeployment.

**Account rename**: treated as a destructive operation. Running deployments are unaffected (pods pull from fully-resolved ECR URLs). CLI credentials and stored deployment specs reference the old name and require manual re-authentication / fresh deployment respectively.

## Migration

- Schema: `ALTER TABLE agent_versions ADD COLUMN ecr_namespace text` with backfill from owning account name. Non-breaking, additive.
- No changes to `accounts`, `agents`, registry proxy, or CLI.
- Existing deployments resolve identically — backfilled `ecr_namespace` matches the current account name which matches the existing ECR path.
