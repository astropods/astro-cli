# Fix agent_card_json 500 and surface AGENT.md hints

## Summary

Registering an agent without an `AGENT.md` file caused a Postgres 500 error because an empty string was inserted into a `json` column. This change fixes the crash and adds end-to-end feedback so users know they're missing the file.

## Design

The root cause was `buildAgentCardJSON` returning `""` when no readme is provided. An empty string is not valid JSON, so Postgres rejects it on insert into the `agent_card_json` column.

**Server fixes (astro-server):**

- `buildAgentCardJSON` now returns `"null"` (valid JSON) instead of `""` for all early-return paths (no readme, parse error, marshal error).
- `Index.Register` applies a defensive guard that normalizes an empty `agentCardJSON` to `"null"` before the SQL insert, protecting against any future caller.
- The registration handler includes a `hints` array in the 201 response when `readme` is empty, telling the caller to add an `AGENT.md`.

**CLI fix (astro-cli):**

- `registerAgent` now parses the 201 response body on every successful push (not just in verbose mode) and prints any `hints` entries as yellow warnings to stderr.

## Migration

No migration required. The server change is backward-compatible — agents already registered with a valid `AGENT.md` are unaffected. The CLI hint is informational only and does not change the push outcome.
