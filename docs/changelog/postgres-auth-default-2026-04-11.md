# Add default auth config to Postgres provider

## Summary

The Postgres provider required users to manually add a `POSTGRES_PASSWORD` input to their `astropods.yml` for the container to start. Without it, Postgres fails with "superuser password is not specified." Every other self-hosted knowledge provider either needs no auth (Qdrant, Redis) or explicitly disables it (Neo4j sets `NEO4J_AUTH=none`).

## Fix

Added `POSTGRES_HOST_AUTH_METHOD=trust` to the Postgres provider's `DefaultEnv`, matching the Neo4j precedent. Knowledge databases are internal-only services that are never exposed outside the cluster — the password requirement adds no security value.

## Migration

No migration required. Existing agents with a `POSTGRES_PASSWORD` input can remove it. New agents using `provider: postgres` will work without any auth configuration.
