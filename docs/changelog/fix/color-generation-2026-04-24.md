# Fix: on-demand color theme generation for blueprints and deployments

## Summary

Blueprints and deployments with no existing `avatar_colors` were never getting a color theme generated. The on-demand `EnsureCurrent` calls in API handlers were guarded by nil checks that skipped entities with `NULL` colors entirely, and the separate River batch backfill job that was meant to cover this gap failed in production.

## Design

Two complementary paths now ensure every blueprint and deployment gets a color theme:

**On-demand generation (API read path):** `EnsureCurrent` is now called unconditionally (when `avatarStore` is configured) in `ListAgents`, `ListAccountAgents`, `GetAgent`, `ListDeployments`, and `GetDeployment`. Previously, a `if agent.AvatarColors != nil` / `if len(...) > 0` guard prevented the call for entities with no colors at all. `EnsureCurrent` already handles nil input correctly — it reads the avatar, extracts colors, persists them, and returns the fresh JSON.

**Eager generation (backfill path):** When the avatar backfill worker generates a new placeholder image or copies one to a deployment, it now extracts and stores colors inline rather than deferring to a separate batch pass. This avoids the extra S3 read that a separate color backfill step would require.

The separate `backfillBlueprintColors` and `backfillDeploymentColors` River job steps have been removed — color extraction is now handled either eagerly at avatar creation time or lazily on first API read.

## Migration

No action required. Existing blueprints/deployments missing colors will have them generated on next API read.
