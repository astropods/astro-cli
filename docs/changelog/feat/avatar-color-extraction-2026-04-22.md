# Avatar color extraction — server-side pipeline

## Summary

Avatar colors for blueprints, deployments, and accounts were previously extracted client-side using canvas + MMCQ on every page view. This was fragile (CORS issues, double image loads, flash of colorless content) and redundant. This change moves color extraction to the server so colors are computed once at avatar mutation time, stored in the database, and served via the API.

## Design

A new `internal/colorextract` package ports the MMCQ (Modified Median Cut Quantization) palette extraction and card color derivation algorithms from `packages/astro-trading-card` to Go. The implementation produces identical results — verified by a cross-validation test that compares Go and TypeScript output for the same deterministic identity avatar.

The extracted color scheme (`AvatarColors`) includes eight derived colors: `base`, `vibrant`, `vibrant_light`, `accent`, `accent_light`, `background`, `foreground`, and `glow`. These are stored as a nullable `avatar_colors` JSONB column on the `agents`, `deployments`, and `accounts` tables.

Color extraction hooks into all avatar mutation points:

- **Blueprints**: `CreateBlueprint` (placeholder generation), `UploadBlueprintAvatar`, `ResetBlueprintAvatar`
- **Deployments**: `DeployAgent` (copies colors from source blueprint), `UploadDeploymentAvatar`, `ResetDeploymentAvatar`
- **Accounts**: `UploadAvatar`, `SetAvatarPreset`, `ResetAvatar`

Extraction is synchronous (sub-millisecond for a 64x64 downsampled image) and non-fatal — failures are logged but never block the primary operation.

Existing entities are backfilled by extending the periodic River workers: `BlueprintAvatarBackfillWorker` gains `backfillBlueprintColors` and `backfillDeploymentColors` phases, and `AvatarBackfillWorker` gains `backfillAccountColors`. All use cursor-paginated batches with `WHERE avatar_colors IS NULL` so the first run fills everything and subsequent runs are no-ops.

The `avatar_colors` field is included in API responses for `AgentResponse` (blueprints) and `AgentDeployment` (deployments) as `omitempty` JSON, so it appears once backfilled without breaking existing clients.

## Migration

No manual migration required. Atlas will apply the schema diff (three new nullable JSONB columns) automatically. The backfill workers populate existing rows on their next periodic run.
