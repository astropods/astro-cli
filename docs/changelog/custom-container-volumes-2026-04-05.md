# Persistent Volumes for Custom Knowledge Containers

## Summary

Knowledge entries using custom containers (not built-in providers) could not use `persistent: true` because the volume mount path was resolved exclusively from the provider registry, which returns empty for custom containers. Docker rejected the empty mount target with a cryptic error. This change adds a `volume` field to the container config so custom containers can specify their data directory, enabling persistent storage for any database image.

## Design

A new `Volume` string field is added to `ContainerConfig` in the spec. When a knowledge entry has `persistent: true`, the compose builder now checks three sources for the mount path in priority order:

1. `container.volume` — user-specified path (new)
2. Provider registry `MountPath` — for built-in providers (existing)
3. Error — if neither is set, `BuildProject` returns a clear error: `persistent is true but no volume path specified`

This same priority is applied in the deployment store (normalized.go) and Kubernetes StatefulSet builder (statefulset.go) so the behavior is consistent across local dev and production deployments.

**Example — custom Postgres with pgvector:**
```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
      volume: /var/lib/postgresql/data
    persistent: true
```

Built-in providers are unaffected — their mount paths still come from the provider registry. The `volume` field is only needed for custom containers.

## Migration

No breaking changes. Existing specs using built-in providers with `persistent: true` continue to work unchanged. Custom containers that previously failed with `persistent: true` can now add `volume` to fix the issue.
