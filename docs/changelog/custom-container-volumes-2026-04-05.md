# Custom Knowledge Container Improvements

## Summary

Two fixes for custom knowledge containers (those using `container` instead of a built-in `provider`):

1. **Persistent volumes** — `persistent: true` now works with custom containers via a new `volume` field that specifies the data directory inside the container.
2. **Environment passthrough** — `environment` on custom knowledge containers is now correctly passed to the running container. Previously these values were silently ignored.

## What's new

### `volume` field on `ContainerConfig`

Custom containers can now declare where persistent data should be stored:

```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
      volume: /var/lib/postgresql/data    # new — tells the platform where data lives
      environment:
        POSTGRES_DB: my_database
    persistent: true
    inputs:
      - name: POSTGRES_PASSWORD
        datatype: string
        secret: true
```

For built-in providers (`provider: qdrant`, `provider: postgres`, etc.), the volume path is already known and `volume` is not needed.

### `environment` now reaches the container

Static configuration like database names, log levels, and feature flags can be set via `environment` on the container. For sensitive values (passwords, API keys), use `inputs` with `secret: true` instead — these are prompted via `ast configure` and stored securely.

## Migration

No breaking changes. Existing specs are unaffected. Custom containers that previously failed with `persistent: true` can now add `volume` to enable persistence.
