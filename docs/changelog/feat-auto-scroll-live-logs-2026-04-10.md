# Live Log Streaming

## Summary

Adds a live log streaming mode to the deployment logs view. Clicking the Live button opens a persistent SSE connection that backfills the last 15 minutes of history server-side and then tails new lines in real time. Reconnects resume from the last received event without re-fetching history, so the view stays stable if the connection drops or the WebSocket read deadline fires on a quiet stream.

## Design

### Backend — `GET /api/v1/deployments/:id/logs/stream`

The endpoint accepts `workload`, `container`, and `pod` query params and returns `text/event-stream`. Two backend paths share the same SSE wire format.

Every emitted log event carries an `id:` field set to the log line's nanosecond Unix timestamp. On reconnect the browser automatically sends this back as `Last-Event-ID`; the server uses it to resume from the cursor instead of replaying history.

**Loki path** (production):

On fresh connection (no `Last-Event-ID`): fetches the last 15 minutes via `QueryLogs`, emits each line as a data event, then dials Loki's WebSocket tail starting at `lastBackfillLine+1ns`. Starting the tail immediately after the last backfill timestamp closes the gap that would otherwise exist between when `QueryLogs` ran and when the tail WebSocket connected. `event: ready` is emitted once the tail WebSocket is open.

On reconnect (`Last-Event-ID` present): skips the backfill entirely and opens the tail from `cursor+1ns`.

**K8s fallback** (local dev / no Loki):

On fresh connection: sets `SinceTime=now-15min` to stream the last 15 minutes.

On reconnect: sets `SinceTime` from the `Last-Event-ID` cursor. Because the K8s API serialises `SinceTime` at second-level precision, lines at or before the cursor are filtered server-side to prevent duplicates.

```
Frontend (EventSource) → GET /deployments/:id/logs/stream
  ├── Loki: QueryLogs (backfill) → TailLogs WebSocket (tail from lastLine+1ns)
  └── K8s: CoreV1().Pods().GetLogs(Follow=true, SinceTime=now-15min or cursor)
```

### Frontend

`useDeploymentLogsStream` manages the `EventSource` lifecycle and accumulates lines via a reducer. On fresh connection it starts with an empty line buffer; the server streams the backfill before emitting `event: ready`. `LogsTab` shows the existing historical logs (from the polling hook) until `ready` fires — at which point the stream already contains the full backfill — then switches cleanly to the stream with no visible flash or line-count jump. No client-side seeding or timestamp filtering is needed.

Live mode is disabled when switching container tabs so each tab loads its own fresh stream on enable.

`LogViewer` additions:
- **Auto-scroll**: follows new lines as they arrive unless the user has scrolled up.
- **Jump to bottom**: floating button restores auto-scroll when the user is scrolled up.
- **Live button deactivates on time-range open**: opening the time selector automatically exits live mode.

## Migration

No changes required. The live toggle opens an SSE stream rather than polling. Operators running behind a proxy should verify `WRITE_TIMEOUT=0` so SSE connections are not closed by the server's default write timeout.
