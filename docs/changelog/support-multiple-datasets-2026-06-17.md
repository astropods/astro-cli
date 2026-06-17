## Summary

Eval dataset rows now have their own UUID identity instead of using `deployment_id` as the primary key. This keeps the current deployment-scoped behavior intact while giving future work a stable dataset identifier for relationships such as judgments or multiple datasets per deployment.

## Design

The `eval_datasets` table now uses `id uuid DEFAULT gen_random_uuid()` as its primary key. `deployment_id` remains unique and continues to cascade with deployments, so current reads and deploy-time provisioning still address the one dataset associated with a deployment.

The server-side dataset store now scans the new ID field when reading rows. Existing create and repoint flows continue to target `deployment_id`, preserving current behavior while allowing later schema changes to reference the dataset row directly.

## Migration

No user action is required. Existing `eval_datasets` rows receive generated UUIDs when the schema migration adds the new column, and the unique deployment constraint preserves current one-dataset-per-deployment semantics.
