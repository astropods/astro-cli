# Custom Container Improvements

## Summary

Two improvements for custom containers (those using `container` instead of a built-in `provider`):

1. **Persistent volumes** — `persistent: true` now works with custom containers via a new `volume` field that specifies the data directory inside the container.
2. **Inputs injection** — `inputs` declared on knowledge, tool, and ingestion entries are now injected into the container at runtime during `ast dev`. Values are resolved from `ast configure` / `.env` with `default` as fallback.

## What's new

### `volume` field on `ContainerConfig`

Custom containers can now declare where persistent data should be stored:

```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
      volume: /var/lib/postgresql/data
    persistent: true
    inputs:
      - name: POSTGRES_DB
        datatype: string
        default: my_database
      - name: POSTGRES_PASSWORD
        datatype: string
        secret: true
        description: Database superuser password
```

For built-in providers (`provider: qdrant`, `provider: postgres`, etc.), the volume path is already known and `volume` is not needed.

### Component inputs now reach their containers in `ast dev`

`inputs` on knowledge, tool, and ingestion entries are now injected as environment variables into those containers during local development. Previously, only agent-level and top-level inputs were injected — component-specific inputs were silently ignored in `ast dev`.

This brings `ast dev` in line with production deployments, where component inputs have always been injected.

## Migration

No breaking changes. Existing specs are unaffected. Custom containers that previously failed with `persistent: true` can now add `volume` to enable persistence.
