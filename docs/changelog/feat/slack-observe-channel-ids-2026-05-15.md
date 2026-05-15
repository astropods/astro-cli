# Slack `observe_channel_ids` configuration

## Summary

The messaging sidecar gained an `observe_channel_ids` setting on the Slack
adapter, which lists channels whose top-level (non-mention) messages should be
forwarded to the agent instead of dropped. The platform did not yet surface
this field, so deployers had no way to configure it from the deploy UI or via
`astropods.yml`.

## Design

`observe_channel_ids` is wired through the two layers that own Slack adapter
configuration:

- **Spec (`packages/astro-spec`)** — added as a named field on
  `SlackAdapterConfig` alongside `allowed_channel_ids` / `allowed_user_ids`,
  plus matching JSON Schema entry and known-keys registration so it round-trips
  cleanly through YAML/JSON. Previously it would have been silently captured
  under `Extra`, which works but bypasses schema validation and IDE
  completion.

- **Server form schema (`apps/astro-server`)** — added a CSV-typed entry to
  the `SLACK_CONFIG` variable's `Fields` map in `injectSlackVariables`. The
  frontend renders this schema dynamically, so no client-side changes are
  needed.

Tests were extended in both packages to cover the new field across spec
parsing and deploy template shaping.

## Migration

No action required. The field is optional and omitted by default.
