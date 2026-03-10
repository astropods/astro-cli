# Fix `ast dev --local`

## Summary

Fixed multiple bugs preventing `ast dev --local` from working, and added convenience improvements to the local development workflow.

## Changes

**Path fix** — `localAstroPackages` and SDK build paths referenced `packages/messaging/...` and `packages/adapters/...`, but submodules live under `modules/`. Updated all three package paths and both SDK check paths in `dev.go`.

**Image tag mismatch** — `moon run deployment:messaging` built `astro-messaging:latest` but `ImageNameForLocal` stripped `astropods/messaging:latest` to `messaging:latest`. Added secondary `-t messaging:latest` and `-t playground:latest` tags to `deployment/moon.yml` so both names resolve.

**Image stripping scope** — `ImageNameForLocal` was applied to all compose services, incorrectly stripping `qdrant/qdrant:latest` to `qdrant:latest`. Now only strips images with the `astropods/` prefix and clears their `pull_policy` so compose uses local builds without attempting a pull.

**`--pull` flag syntax** — `docker compose build --pull=never` fails on current compose versions because `--pull` is a boolean flag on the `build` subcommand. Inverted the logic: pass `--pull` only when pulling is desired (default without the flag is no-pull).

**Auto-build SDKs** — Replaced the "check dist/ and error" pattern with automatic `bun install && bun run build` for each SDK when `dist/index.js` is missing. Covers `@astropods/messaging`, `@astropods/adapter-core`, and `@astropods/adapter-mastra`.

**`ast dev logs` default** — Previously defaulted to the `agent` service, which doesn't exist in `--local` mode. Now defaults to all services when no argument is provided.

**Error messages** — `ASTRO_ROOT` errors now include an example `export` command.

**PR changelog action** — `pr-changelog.yml` now searches `docs/changelog/` recursively, so branches with slashes (e.g. `fix/my-change`) can use matching subdirectories (e.g. `docs/changelog/fix/my-change-YYYY-MM-DD.md`). Flat filenames continue to work as before. Updated `CLAUDE.md` guidance to reflect both conventions.

## Migration

No action required. Existing `moon run deployment:*` tasks still produce the same primary tags; the new secondary tags are additive.
