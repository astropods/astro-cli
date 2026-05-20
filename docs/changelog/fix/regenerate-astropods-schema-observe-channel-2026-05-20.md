# Regenerate astropods schema for observe_channel_ids

## Summary

`observe_channel_ids` was added to `SlackAdapterConfig` and `DevInterfaces` JSONSchema in Go, but `packages/astro-spec/astropods.schema.json` was not regenerated. Installed CLIs (e.g. 0.13.1) embed that file, so `ast push` rejected specs using `dev.interfaces.messaging.slack.observe_channel_ids` with "additional properties not allowed".

## Design

Run `go generate` in `astro-spec` so the committed `astropods.schema.json` matches the reflector output from `spec.go` (including `observe_channel_ids` under `dev.interfaces.messaging.slack`). Bump `apps/astro-cli/VERSION` to **0.13.2** and release via `Release CLI (Prod)` so `ast upgrade` picks up the fixed validator.

## Migration

After 0.13.2 is released: `ast upgrade`, then `ast push` as usual. No spec changes required if YAML already includes `observe_channel_ids`.
