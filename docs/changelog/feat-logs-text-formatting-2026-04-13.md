# Structured Log Formatting

## Summary

Log lines previously arrived at the frontend as raw plain-text strings with a prepended timestamp, requiring client-side regex parsing to extract structure. This change replaces that with a structured JSON response from the server so the frontend receives typed `{ timestamp, level, message }` objects directly.

## Design

### Server

The logs endpoint (`GET /api/v1/deployments/:id/logs`) and the knowledge store logs endpoint (`GET /api/v1/knowledge/:name/logs`) both now return `application/json` instead of `text/plain`. The response is an array of log entry objects:

```json
[
  { "timestamp": "2026-04-13T21:48:08.470Z", "level": "info", "message": "agent started" }
]
```

The shared `streamLogs` helper in `handlers/logs.go` handles both response paths. The `Level` field is populated from the `level` Loki stream label when present (empty string otherwise). The K8s fallback path parses the RFC3339 timestamp K8s prepends with `Timestamps: true` and extracts it into the structured field.

The Loki `LogLine` struct gains a `Level` field that is read from `stream.Stream["level"]`. This will be empty until the Alloy pipeline is updated to extract level as a stream label.

### Client

`LogEntry` is the new shared type (`src/lib/log-utils.ts`):

```ts
interface LogEntry { timestamp: string | null; level: string | null; message: string; }
```

Level values from the server are normalised through `LEVEL_MAP` which maps raw strings (`warn`, `warning`, `err`, `crit`, etc.) to canonical `LogLevel` values. Unknown values and absent levels default to `INFO`.

`LogViewer` accepts `LogEntry[]` instead of `string[]`. `useLogFiltering` uses the normalised level for error/warning badge counts and filtering — no regex scanning of message text.

### CLI

`ast knowledge logs` previously streamed the response as plain text line by line. It now decodes the JSON array via the shared `printLogs` helper (`cmd/logs.go`), which can be reused by any future log command.

## Migration

No action required for users. The `ast knowledge logs` CLI command output format is unchanged (timestamp, level, message per line). The Alloy pipeline change to extract `level` as a stream label is a separate follow-up in `astro-infra` — until that lands, all log lines will display as `INFO`.
