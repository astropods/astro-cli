# Upgrade Postgres Provider to pgvector

## Summary

The built-in `postgres` knowledge provider used `postgres:15-alpine`. This upgrades it to `pgvector/pgvector:pg17`, which is a standard PostgreSQL 17 image with the pgvector extension pre-installed. Agents using `provider: postgres` now get vector search support out of the box without needing a custom container.

## Design

One-line image change in the provider registry. The pgvector image is a superset of the standard postgres image — fully compatible, same port, same data directory, same health check. The extension is inert unless explicitly enabled with `CREATE EXTENSION vector`, so existing postgres users are unaffected.

This is particularly relevant for AI agent workloads where semantic vector search is a common need alongside traditional relational storage.

## Migration

No breaking changes. Existing agents using `provider: postgres` will get the new image on their next deployment. Data format is compatible — PostgreSQL 17 reads PostgreSQL 15 data directories.
