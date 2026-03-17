# Migrate ast dev to Docker Compose v5 SDK

## Summary

The `ast dev` command previously orchestrated local containers by shelling out to the `docker compose` CLI binary, which required it to be installed separately and made error handling brittle. This migrates all compose operations to the in-process `docker/compose/v5` Go SDK, giving the CLI direct programmatic control over build, up, down, logs, and one-off container runs.

## Design

### Compose SDK migration

All compose operations (`build`, `up`, `down`, `logs`, `run`) now go through a single `newComposeService(verbose bool)` factory that initialises a `docker/compose/v5` service backed by the local Docker daemon. In non-verbose mode the Docker CLI's progress stream is discarded; in verbose mode a `loggingEventProcessor` prints per-resource status events (pull, build, create, start, stop) with icons to stdout.

Key behavioural changes:

- **Build** — all services, including profiled ingestion containers, are built upfront in a single `svc.Build` call. The project is given `Profiles: ["ingestion"]` during the build pass so the SDK resolves profiled services by name; `Up` is then called on a profile-free copy so only non-ingestion services start.
- **State file** — instead of writing `.ast/docker-compose.yml` to disk, `ast dev` writes `.ast/.running` containing the project name. Subcommands (`stop`, `logs`, `trigger`) read this file to resolve the project without re-parsing the spec. An older empty-file format falls back to spec parsing.
- **Spinners** — a `withSpinner` helper runs any operation behind a Charmbracelet spinner in non-verbose mode, or prints inline progress in verbose mode.
- **Trigger** — `ast dev trigger` runs the ingestion as a one-off container via `svc.RunOneOffContainer` with `AutoRemove: true`. Startup ingestions run the same way automatically after `Up` completes.
- **Logs** — `ast dev logs [service]` streams container logs through `svc.Logs` with a `stdoutLogConsumer`, with Ctrl+C handled via context cancellation.

Container images produced by `BuildProject` are tagged with the required compose SDK labels (`com.docker.compose.project`, `com.docker.compose.service`, etc.) so the SDK can discover running containers by project name rather than relying on a compose file on disk.

## Migration

No user-facing changes to the spec format. Users must have Docker Desktop (or the Docker daemon) running — the SDK connects to the local daemon directly. The `docker compose` CLI binary is no longer required for `ast dev`.

Existing `.ast/docker-compose.yml` files from previous dev runs are inert and can be deleted.
