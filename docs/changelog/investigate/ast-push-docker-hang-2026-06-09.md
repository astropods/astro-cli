## Summary

`ast push` started failing on Docker Desktop 4.77.0 with `archive/tar: invalid tar header` during the BuildKit "load remote build context" step. Our build path (`apps/astro-cli/cmd/build_runner.go`) builds a tar of the project context with `moby/go-archive` and posts it to the engine's `/build` endpoint, which now hands the stream to a much stricter BuildKit tar parser. The astro-cli was pinned to engine/client/buildkit versions that were one to three releases behind what Docker Desktop ships, so the tar producer was emitting headers the new receiver rejects.

This change re-aligns the docker/moby module set with what Docker Desktop 4.77.0 actually runs.

## Design

| Dependency                     | Before                  | After                   | Why                                          |
|--------------------------------|-------------------------|-------------------------|----------------------------------------------|
| `github.com/docker/docker`     | v28.5.2+incompatible    | **dropped**             | Sole remaining import migrated to `moby/moby/api` |
| `github.com/docker/cli`        | v29.2.1+incompatible    | v29.5.3+incompatible    | Matches Desktop 4.77.0 engine exactly        |
| `github.com/docker/compose/v5` | v5.1.0                  | v5.1.4                  | Matches Desktop 4.77.0                       |
| `github.com/moby/buildkit`     | v0.27.1                 | v0.29.0                 | Compose v5.1.4 requires v0.29.0; v0.30.0 dropped `util/tracing/env` which compose still imports |
| `github.com/moby/moby/api`     | v1.53.0                 | v1.54.2                 | Matches engine API v1.54                     |
| `github.com/moby/moby/client`  | v0.2.2                  | v0.4.1                  | Goes with the v1.54 API package              |
| `github.com/moby/go-archive`   | v0.2.0                  | v0.2.0                  | Already latest                               |

Code change is a one-line import migration in `apps/astro-cli/cmd/push_streaming.go` — the lone `docker/docker/api/types/registry` reference (used only for `registry.AuthConfig`) is now sourced from `moby/moby/api/types/registry`. With that move the `github.com/docker/docker` direct dependency disappears entirely.

`moby/buildkit` is intentionally pinned to **v0.29.0** rather than the latest v0.30.0. Compose v5.1.4 (what Desktop now ships) still imports `github.com/moby/buildkit/util/tracing/env`, a package that was removed in buildkit v0.30.0. Until compose drops that import, v0.29.0 is the highest version we can take.

CLI patch bump: **0.15.1 → 0.15.2**.

## Migration

None for users — the CLI is statically built. Anyone on Docker Desktop 4.77.0 hitting the tar-header error should `ast upgrade` (or rebuild) once `0.15.2` is published. Developers re-running `go mod tidy` will see the same module set described above.
