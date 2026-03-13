# Skip Slack adapter when token is missing during ast dev

## Summary

`ast dev` crashed the messaging service on every freshly created agent. The `ast create` TUI credentials prompt tells users `"leave blank to skip"`, implying they can configure tokens later. But when the Slack adapter is in the spec (the default scaffold includes `adapters: [web, slack]`), the compose builder unconditionally set `SLACK_ENABLED=true` regardless of whether `SLACK_BOT_TOKEN` was present. The messaging service then hard-failed on startup with `SLACK_BOT_TOKEN is required when Slack is enabled`, killing the entire dev environment — including the web playground.

## Design

The compose builder now checks for `SLACK_BOT_TOKEN` in the env vars before setting `SLACK_ENABLED=true`. When the token is absent, it prints a warning (`⚠ Slack adapter listed but SLACK_BOT_TOKEN not set — skipping`) and continues with just the web adapter. The messaging service starts normally and the playground works.

## Migration

No action required. Agents with Slack tokens configured continue to work as before.
