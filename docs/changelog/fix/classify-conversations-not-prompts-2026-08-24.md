# Classification reads conversations, not single prompts

## Summary

The purpose head labelled roughly half of some developers' Claude Code prompts `personal`. Purpose and topic are trained on whole conversations, production sent them one prompt at a time, and the training corpus taught the model that a terse contextless turn is casual use — two thirds of real Claude Code turns are under 120 characters. On the in-domain eval set, the same checkpoint scores 2.1% `personal` per conversation and 17.4% per prompt against a 1.6% truth.

## Design

A day's prompts are grouped by `trace.sessionId`, joined oldest-first, and classified as one text. Rows stay one-per-prompt, each carrying its conversation's verdict: the day's cost is apportioned by row count, so a row per conversation would weigh a 20-prompt session the same as a single question. No migration, and fewer inference calls than before.

Only `claude_code.interaction` is classified now. The `claude-code` tag also matches the `tool_result`, `assistant_response` and `claude_code.llm_request` records astro-otel synthesizes — 41% of everything previously sent to inference.

Session ids were absent because `OTEL_METRICS_INCLUDE_SESSION_ID` was `false`. The setting gates logs and traces too despite its name, and `session.id` lands on metric datapoint attributes, never on resource attributes — so astro-otel's existing resource-level strip was a no-op. `stripDatapointAttr` clears all five datapoint kinds, which is what makes the flag safe to turn on.

## Migration

None in the schema. **Deploy astro-otel before the settings block**, or session ids reach VictoriaMetrics as unbounded series labels.

Existing accounts need an admin to re-paste the block in their Anthropic console; developers pick it up on their next session. Until then a prompt with no session id is a conversation of one, which is the previous behaviour, so coverage holds. Existing rows keep their per-prompt labels and are superseded as days re-run.
