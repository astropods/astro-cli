# Remove redundant .npmrc from scaffold; skip Slack adapter when token is missing

## Summary

Two fixes to `ast create` / `ast dev`:

1. The scaffold generated a `.npmrc` file (`@astropods:registry=https://registry.npmjs.org`) that serves no purpose — `registry.npmjs.org` is already the default npm registry. Removed from scaffold, Dockerfile templates, and `ast repair`.

2. `ast dev` crashed the messaging service on every freshly created agent. The `ast create` TUI credentials prompt tells users `"leave blank to skip"`, implying they can configure tokens later. But when the Slack adapter is in the spec (the default scaffold includes `adapters: [web, slack]`), the compose builder unconditionally set `SLACK_ENABLED=true` regardless of whether `SLACK_BOT_TOKEN` was present. The messaging service then hard-failed on startup with `SLACK_BOT_TOKEN is required when Slack is enabled`, killing the entire dev environment — including the web playground.

## Design

**`.npmrc` removal** — deleted `npmrc.tmpl`, removed the `Npmrc` field from `TemplatePaths`, dropped `COPY .npmrc` from both Dockerfile templates (`Dockerfile` and `Dockerfile.ingestion`), removed the `.npmrc` entry from `scaffold.go`, `repair.go`, and the associated bun.lock cleanup step. Updated the Python starter spec doc to remove `.npmrc` from the TypeScript file list.

**Slack adapter guard** — the compose builder now checks for `SLACK_BOT_TOKEN` in the env vars before setting `SLACK_ENABLED=true`. When the token is absent, it prints a warning (`⚠ Slack adapter listed but SLACK_BOT_TOKEN not set — skipping`) and continues with just the web adapter. The messaging service starts normally and the playground works.

## Migration

No action required. Existing agents with a `.npmrc` file are unaffected — the file is harmless, it just won't be generated or repaired anymore. Agents with Slack tokens configured continue to work as before.
