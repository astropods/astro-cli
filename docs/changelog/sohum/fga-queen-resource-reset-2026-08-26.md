# Guarded authorization resource reset

## Summary

Queen can now dry-run and delete one Astro account's WorkOS product resources. This prepares that account for rebuilding the authorization hierarchy with Account as the practical parent.

Queen labels this action **Reset FGA resources** so it cannot be confused with deleting the Astro account or its WorkOS organization.

## Design

The reset is a durable River operation enabled explicitly per environment:

1. Queen selects an Astro account. The server resolves its WorkOS organization and records a dry run containing only that account's product resources.
2. Destructive reset requires that count and is enabled only when `FGA_AUTHORIZATION_RESET_ENABLED=true`.
3. The worker removes direct assignments and deletes the selected account's product resources. It never targets the WorkOS organization root or another account's resources.
4. Queen shows progress and errors; no reconstruction or follow-up action runs automatically.

The operation ledger makes retries and partial failure visible without pausing FGA lifecycle work.
Its rows cascade with account deletion so reset history never blocks normal account cleanup.
Postgres permits only one queued or running reset per account, and retries revalidate the confirmed count until the first deletion succeeds.

## Migration

Apply the `authorization_admin_operations` schema, including its required `account_id`, before enabling reset. Environments without the explicit reset flag remain read-only.
