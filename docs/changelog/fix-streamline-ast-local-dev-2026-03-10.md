# Fix `ast dev --local`

## Summary

Fixed multiple bugs preventing `ast dev --local` from working, and added convenience improvements to the local development workflow.

## Changes

**Path fix** — `localAstroPackages` and SDK build paths referenced `packages/messaging/...` and `packages/adapters/...`, but submodules live under `modules/`. Updated all three package paths and both SDK check paths in `dev.go`.

**Image tag mismatch** — `moon run deployment:messaging` built `astro-messaging:latest` but `ImageNameForLocal` stripped `astropods/messaging:latest` to `messaging:latest`. Renamed tags in `deployment/moon.yml` from `astro-messaging:latest`/`astro-playground:latest` to `messaging:latest`/`playground:latest` to match.

**Image stripping scope** — `ImageNameForLocal` was applied to all compose services, incorrectly stripping `qdrant/qdrant:latest` to `qdrant:latest`. Now only strips images with the `astropods/` prefix and clears their `pull_policy` so compose uses local builds without attempting a pull.

**`--pull` flag fix** — `docker compose build` treats `--pull` as a boolean flag, so `--pull=never` was invalid. Changed to pass `--pull` in normal mode (always pull base images) and `--pull=false` in `--local` mode (explicitly skip pulling).

**Auto-build SDKs** — Replaced the "check dist/ and error" pattern with automatic `bun install && bun run build` for each SDK when `dist/index.js` is missing. Covers `@astropods/messaging`, `@astropods/adapter-core`, and `@astropods/adapter-mastra`.

**`ast dev logs` default** — Previously defaulted to the `agent` service, which doesn't exist in `--local` mode. Now defaults to all services when no argument is provided.

**Error messages** — `ASTRO_ROOT` errors now include an example `export` command.

**PR changelog action** — `pr-changelog.yml` now searches `docs/changelog/` recursively, so branches with slashes (e.g. `fix/my-change`) can use matching subdirectories (e.g. `docs/changelog/fix/my-change-YYYY-MM-DD.md`). Flat filenames continue to work as before. Updated `CLAUDE.md` guidance to reflect both conventions.

**Extracted `composeBuildArgs`** — Pulled the `docker compose build` argument construction out of `runDevStart` into a standalone `composeBuildArgs()` function for testability.

**Tests** — Added `TestComposeBuildArgs` in `cmd/dev_test.go` covering normal/local/rebuild/combined modes, asserting `--pull` vs `--pull=false` presence and mutual exclusivity, and `--no-cache` presence. Extended `utils_test.go` with `astropods/`-prefixed image cases and a new `TestImageStrippingStrategy` that verifies only `astropods/*` images are stripped while third-party images like `qdrant/qdrant:latest` pass through unchanged.

## Migration

No action required. If you have scripts referencing `astro-messaging:latest` or `astro-playground:latest`, update them to `messaging:latest` / `playground:latest`.
