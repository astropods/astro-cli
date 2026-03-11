# Fix `ast dev --local`

## Summary

Fixed multiple bugs preventing `ast dev --local` from working, and added convenience improvements to the local development workflow.

## Changes

**Path fix** — `localAstroPackages` and SDK build paths referenced `packages/messaging/...` and `packages/adapters/...`, but submodules live under `modules/`. Updated all three package paths and both SDK check paths in `dev.go`.

**Image tag mismatch** — `moon run deployment:messaging` built `astro-messaging:latest` but `ImageNameForLocal` stripped `astropods/messaging:latest` to `messaging:latest`. Renamed tags in `deployment/moon.yml` from `astro-messaging:latest`/`astro-playground:latest` to `messaging:latest`/`playground:latest` to match.

**Image stripping scope** — `ImageNameForLocal` was applied to all compose services, incorrectly stripping `qdrant/qdrant:latest` to `qdrant:latest`. Now only strips images with the `astropods/` prefix and clears their `pull_policy` so compose uses local builds without attempting a pull.

**`--pull` flag fix** — `docker compose build` treats `--pull` as a boolean flag, so `--pull=never` was invalid. Changed to pass `--pull` in normal mode (always pull base images) and `--pull=false` in `--local` mode (explicitly skip pulling).

**Auto-build SDKs** — Replaced the "check dist/ and error" pattern with automatic `bun install && bun run build` for each SDK when `dist/index.js` is missing. Covers `@astropods/messaging`, `@astropods/adapter-core`, and `@astropods/adapter-mastra`.

**Auto-build Docker images** — `--local` now always builds `messaging:latest` and `playground:latest` from `$ASTRO_ROOT/modules/`. Docker layer caching keeps repeat builds fast. `--rebuild` additionally passes `--no-cache`.

**Orphan cleanup on startup** — Runs `docker compose down --remove-orphans` before `up -d` to clean up leftover containers from a force-killed previous session.

**Graceful shutdown** — Agent process now runs in its own process group (`Setpgid`); shutdown kills the entire group instead of just the shell. `signal.Stop` after first Ctrl+C restores default handling so a second Ctrl+C force-exits. `docker compose down` runs with a 30s timeout.

**Service health check** — After starting services, prints color-coded status for each container (running/exited/unknown).

**Background log streaming** — Docker compose logs are streamed alongside the agent output so service failures are visible without a separate terminal.

**`ast dev logs` default** — Defaults to the `agent` service. Added `--all` flag to tail all services.

**Error messages** — `ASTRO_ROOT` errors now include an example `export` command.

**PR changelog action** — `pr-changelog.yml` now searches `docs/changelog/` recursively, so branches with slashes (e.g. `fix/my-change`) can use matching subdirectories (e.g. `docs/changelog/fix/my-change-YYYY-MM-DD.md`). Flat filenames continue to work as before. Updated `CLAUDE.md` guidance to reflect both conventions.

**Extracted helpers** — Pulled `composeBuildArgs()`, `devLogsArgs()`, `killProcessGroup()`, `checkComposeHealth()`, and `buildLocalImages()` into standalone functions for testability and clarity.

**Tests** — Added `TestComposeBuildArgs`, `TestDevLogsArgs`, `TestLocalAstroPackagesPointToModules`, `TestLocalDockerImagesConsistency`, and `TestResolveAstroSourceRoot` in `cmd/dev_test.go`. Extended `utils_test.go` with `astropods/`-prefixed image cases and `TestImageStrippingStrategy`.

**Docs** — Updated `cmd/docs/ast.md`, `docs/02-cli/cli-design.md`, and `apps/astro-cli/README.md` to reflect `--all` flag, `moon run astro-cli:link`, and config namespacing.

**Local dev CI action** — Added `.github/workflows/local-dev-check.yml` that runs the local-dev regression tests and verifies the CLI builds with production ldflags on PRs touching `apps/astro-cli/`, `deployment/moon.yml`, or `.gitmodules`. No secrets or submodules required.

## Migration

No action required. If you have scripts referencing `astro-messaging:latest` or `astro-playground:latest`, update them to `messaging:latest` / `playground:latest`.
