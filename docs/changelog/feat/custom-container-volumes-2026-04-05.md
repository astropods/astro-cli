# Custom Container Improvements

## Summary

This change addresses three gaps that prevented custom knowledge, tool, and ingestion containers from being fully configurable and persistent during local development with `ast dev`.

1. **Persistent volumes for custom containers** — `persistent: true` previously failed for any container-mode knowledge entry because the volume mount path was resolved exclusively from the built-in provider registry, which has no entry for custom containers. A new `volume` field on `ContainerConfig` lets users specify the mount path, and the compose builder checks it before falling back to the provider registry. A clear error is returned if `persistent` is set but no volume path can be resolved.

2. **Component inputs injected into containers during `ast dev`** — `inputs` declared on knowledge, tool, and ingestion entries are now injected as environment variables into those containers during local development. Previously, `ast dev` only injected agent-level and top-level inputs into the agent and ingestion containers. Component-specific inputs — the same ones surfaced in the deploy UI for production configuration — were silently ignored locally. This meant custom containers that needed configuration (like a Postgres password) could not receive it through the standard `inputs` mechanism during development. The fix applies the same resolution pattern used for agent inputs: values are looked up from the project config store (set via `ast configure`) with `inp.Default` as fallback.

3. **JSON schema updated** — The generated `astropods.schema.json` now includes the `volume` field, so editor autocompletion and `ast validate` accept it.

## What users can now do

### Run a custom database with persistent storage

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
        description: Database name
      - name: POSTGRES_USER
        datatype: string
        default: postgres
      - name: POSTGRES_PASSWORD
        datatype: string
        secret: true
        description: Database superuser password
```

- `volume` tells the platform where data lives inside the container. A named Docker volume is created and mounted at this path, surviving `ast dev stop` and restart cycles.
- `inputs` are injected into the container as environment variables. Run `ast configure` to set values, or rely on `default`. Values marked `secret: true` are stored securely and never logged.
- Built-in providers (`provider: qdrant`, `provider: postgres`, etc.) already know their volume paths and don't need the `volume` field.

### Configure tools and ingestion containers with inputs

```yaml
integrations:
  mcp-server:
    container:
      image: my-mcp:latest
      port: 8080
    inputs:
      - name: API_KEY
        datatype: string
        secret: true
        description: API key for the MCP server

ingestion:
  sync:
    container:
      build:
        context: .
        dockerfile: ingestion/Dockerfile
    trigger:
      type: schedule
    inputs:
      - name: SYNC_BATCH_SIZE
        datatype: number
        default: "100"
        description: Documents per sync batch
```

These inputs work the same way in `ast dev` as they do in production deployments — they are injected as environment variables into the respective container.

## Changes

- `packages/astro-spec/spec.go` — added `Volume` field to `ContainerConfig`
- `packages/astro-spec/astropods.schema.json` — regenerated to include `volume`
- `apps/astro-cli/internal/compose/builder.go` — volume mount resolution with container fallback; inputs injection for knowledge, tool, and ingestion containers
- `apps/astro-server/internal/deploymentstore/normalized.go` — volume path resolution with container fallback for production deployments
- `apps/astro-server/internal/k8s/statefulset.go` — same for Kubernetes StatefulSets
- `apps/astro-cli/internal/compose/builder_test.go` — tests for custom persistent volumes, missing volume error, knowledge inputs injection, tool inputs injection
- `apps/astro-cli/cmd/docs/agent_instructions.md` — new Knowledge Stores section in CLI docs
- `docs-public/fern/docs/pages/astropods-package-spec.mdx` — spec v1.3: `volume` field, inputs guidance, updated examples

## Migration

No breaking changes. Existing specs are unaffected. Custom containers that previously failed with `persistent: true` can now add `volume` to enable persistence. Component inputs that were previously ignored in `ast dev` will now take effect — if any input has a `default`, it will be injected on the next `ast dev` run.
