# Local development docs refresh

## Summary

The run and setup docs across the monorepo (and the astro-cli and messaging submodules) had drifted from the actual local-dev workflow, so following them led to failures on stale commands, ports, and flags. This realigns them with current source and removes the last of the deprecated playground.

## Design

- **Local stack.** Documented the two ways to run locally as distinct options: (A) `./scripts/local-dev.sh` - one command, everything behind Traefik on a single `http://localhost` origin; (B) `moon run astro-server:dev` (:8080) + `moon run astro-client:dev` (:5173), where the Vite dev server proxies `/api` to the backend so the browser stays same-origin. Both need `apps/astro-server/.env` and a running Docker.
- **Remote dev database.** The dev database is remote via `DATABASE_URL`; nothing local starts Postgres. Corrected the astro-server README (dropped the "starts Postgres" claim and the fictional `AUTH_ENABLED`; port is `8080`, not `4321`) and relabeled the `.env.example` `DATABASE_URL` placeholder. Local auth uses the stage/preview WorkOS app, a separate identity from prod.
- **CLI.** The CLI lives at `modules/astro-cli` and builds to `modules/astro-cli/bin/ast-dev`. Removed the deleted `--local`/`--local-reset` flags and the non-existent `ASTRO_ROOT` var from the guides; local mode is inferred from the server URL host. Regenerated the Moon target list and the Apps/Packages tables from source.
- **Playground removal completed.** The messaging container's playground was already removed; this removes the remaining residue: astro-server no longer injects the dead `WEB_SERVE_PLAYGROUND` env var (the messaging binary ignores it), the stale `PlaygroundPanel` example is gone from the astro-client conventions, and the never-shipped in-app-playground spec is deleted.
- **Frontend.** The local astro-server is the only supported target, so `VITE_API_URL` documentation is trimmed to that; removed the remote-backend flow entirely (the README sections, the `scripts/setup-local-dev.sh` / `bun run setup` script, and the `local.astropods.ai` same-site domain + mkcert HTTPS detection in `vite.config.ts`; the dev server now always binds `host: true` and proxies to `VITE_API_URL`). Fixed the messaging README (`STORAGE_TYPE` defaults to `redis`, `SLACK_ENABLED`/`WEB_ENABLED` gate their adapters, dropped the removed `VERSION` file section).

## Migration

None required. Documentation plus one dead-env-var removal; no behavioral change, since the messaging binary already ignored `WEB_SERVE_PLAYGROUND`.
