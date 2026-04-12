# Auto-derive Postgres database name from agent name

## Summary

When using `provider: postgres`, the platform now automatically creates a database named after the agent. Previously, users had to manually add a `POSTGRES_DB` input — without it, Postgres only created the default `postgres` database and the agent's queries would fail with "database does not exist."

## How it works

The agent name is sanitized into a valid Postgres database name (hyphens become underscores, lowercased). For example, an agent named `memory-box` gets a database called `memory_box`.

`POSTGRES_DB` is injected into both:
- The knowledge container (so the Postgres entrypoint creates the database on first init)
- The agent container (so application code knows which database to connect to)

This applies to both `ast dev` (Docker Compose) and production (Kubernetes) deployments.

## Migration

No migration required. Existing agents with a `POSTGRES_DB` input can remove it — the platform handles it automatically. Agents that never declared `POSTGRES_DB` will now get a database created for them instead of failing.
