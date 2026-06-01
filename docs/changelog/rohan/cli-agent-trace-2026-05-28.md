# `ast agent trace`

## Summary

The web client exposes per-agent traces under `Agents → <name> → Monitor → <trace id> → Overview`, but the CLI had no equivalent. Operators debugging from a terminal had to copy a trace ID into a browser tab to see input, output, observations, and scores. This command brings parity.

## Design

`ast agent trace` mirrors the two HTTP endpoints the Monitor tab already calls — there is no new backend surface.

```
ast agent trace --name <name>                    → GET /api/v1/deployments/{id}/observability/traces
ast agent trace --name <name> --trace-id <id>    → GET /api/v1/deployments/{id}/observability/traces/{traceId}
```

Both views share the same target resolution as the rest of the `agent` subcommands: `--name` or `--id` (mutually exclusive), `resolveAgentTarget`, `cmdAuth`, and `apiCall`. Trace detail uses `--trace-id` / `-t` instead of a positional argument so agent names and trace IDs get clear, separate validation and error messages.

List view renders the columns surfaced in the Monitor table — time, trace id, name, latency, cost — and stops short of the full input/output snippets that don't fit cleanly in a terminal. Pagination uses `--limit` / `--offset` with CLI-side validation (`--limit` must be > 0, `--offset` ≥ 0; default limit 50 matches the server). List flags are validated even when `--trace-id` is set (they are ignored in detail mode, but invalid values still error). Time window filters (`--start`, `--end`) are validated as RFC3339 and must be chronological before being sent as `start_time` / `end_time` query params.

Detail view prints the trace header (id, name, time, latency, cost, session, user, tags), pretty-printed metadata, input and output JSON (`indentJSON` in `utils.go`), observations sorted by start time (including per-observation input/output when present), and any scores. User-facing errors live in `messages.go`.

`--json` on either view emits the raw server response unchanged so it can be piped to `jq`.

## Migration

None. New subcommand, no breaking changes. Existing `ast agent` commands are untouched.
