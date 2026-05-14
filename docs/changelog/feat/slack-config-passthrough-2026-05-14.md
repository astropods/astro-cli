# Slack adapter: arbitrary key pass-through to SLACK_CONFIG

## Summary
Users want to configure new messaging-sidecar options under `dev.interfaces.messaging.slack` in `astropods.yml` without waiting for spec changes. Previously, only the five fields declared on `SlackAdapterConfig` survived parsing; anything else was silently dropped before the YAML was serialized into the `SLACK_CONFIG` env var.

## Design
`SlackAdapterConfig` keeps its typed fields (`actionable_reactions`, `allowed_channel_ids`, `allowed_user_ids`, `socket_mode`, `auto_thread`) so jsonschema docs and compile-time access stay intact. An additional `Extra map[string]any` field with `yaml:",inline"` captures every other key. Custom `MarshalJSON` / `UnmarshalJSON` flatten `Extra` alongside the named fields so the JSON written to `SLACK_CONFIG` looks identical to the YAML the user authored. Named fields win on key conflict.

The messaging sidecar's existing `json.Unmarshal` into its own typed `SlackAdapterConfig` continues to ignore unknown keys, so back-compat is preserved on the consumer side until those keys are wired up.

## Migration
None. Existing specs parse and serialize identically; only previously-dropped keys now flow through.
