## Summary

Blueprint name availability check incorrectly blocked reuse of archived names, and was also too permissive — it allowed reuse of names that had been publicly visible or deployed, which is a security risk (a new blueprint could impersonate the public history of an old one).

## Design

A `name_reserved bool NOT NULL DEFAULT false` column is added to the `agents` table as a one-way ratchet. It is set to `true` in two places:

1. **On visibility change to public** — `SetVisibility` sets `name_reserved = (name_reserved OR visibility = 'public')` so the flag is sticky even if the blueprint is later made private.
2. **On first deployment** — the `DeployAgent` handler fires a best-effort goroutine that calls `agentIndex.MarkNameReserved` immediately after `SaveDeploymentPending` succeeds for a new (non-update) deploy.

The field is exposed in `AgentResponse` and the client `Blueprint` type. The availability check becomes:

```ts
// name is taken if: active blueprint, OR archived-but-reserved (was ever public or deployed)
const nameIsTaken = !!existingBlueprint && (!existingBlueprint.archived_at || !!existingBlueprint.name_reserved);
```

Archived blueprints with `name_reserved = false` (never public, never deployed) can be freely reclaimed — the server unarchives and resets them on `Create`.

## Migration

Run before deploying the server image:

```sql
ALTER TABLE agents ADD COLUMN IF NOT EXISTS name_reserved bool NOT NULL DEFAULT false;

-- Backfill: reserve names for agents that are currently public or have ever been deployed.
UPDATE agents SET name_reserved = true
WHERE visibility = 'public'
   OR EXISTS (
       SELECT 1 FROM deployments d
       WHERE d.account_id = agents.account_id AND d.agent_name = agents.name
   );
```
