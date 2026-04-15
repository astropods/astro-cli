# Tail Log Streaming

## Summary

Adds a live tail mode to the deployment logs view. Clicking **Tail** opens an SSE connection that streams new log lines from the moment the connection opens — no history is backfilled. When the server-side stream closes (Loki WebSocket disconnect or pod stream end), the SSE response ends and the browser's `EventSource` reconnects automatically, showing a "Reconnecting…" indicator in the toolbar.

## Design

### Backend — `GET /api/v1/deployments/:id/logs/stream`

Accepts `workload`, `container`, and `pod` query params and returns `text/event-stream`. Both backend paths follow the same pattern: open the log source from `time.Now()`, emit `event: ready`, forward lines until the source closes, then return. Closing the handler ends the SSE response so the browser reconnects.

**Loki path:** dials Loki's WebSocket tail with `Start=time.Now()`. On dial error, emits `event: error` and returns. Otherwise emits `event: ready` and ranges over the channel until it closes.

**K8s fallback:** calls `CoreV1().Pods().GetLogs` with `Follow=true` and `SinceTime=time.Now()`. Emits `event: ready`, then scans lines until the stream ends.

### Frontend

**`LogStreamProvider`** is a React context provider mounted at the `ActiveDetailView` level. It holds a single `EventSource` and manages stream state (`idle → connecting → tailing → reconnecting`). Because the provider lives above the tab strip, the connection survives switching between Monitor, Deployments, and Logs tabs within the same deployment view.

- `startStream(deploymentId, workload, container)` — closes any existing connection, opens a new `EventSource`, accumulates lines (capped at 5000).
- `stopStream()` — closes the connection and resets state.
- Unmount cleanup closes the connection when navigating away from the deployment.

**`LogsTab`** manages container sub-tabs and the `isTailing` toggle. Historical logs use `useDeploymentLogs` (TanStack Query, one-shot fetch) when not tailing. Switching container tabs or turning off Tail reverts to historical mode. An auto-disconnect fires after 30 seconds if the user navigates away from the Logs tab while tailing.

**`LogViewer`** additions:
- **Tail toggle**: pulsing dot when active; time-range selector is disabled while tailing.
- **Reconnecting indicator**: spinner shown while `EventSource` is auto-retrying.
- **Auto-scroll**: follows new lines unless the user has scrolled up.
- **Jump to bottom**: floating button restores auto-scroll.

**SSE status events** — the backend now emits `event: status` frames at key lifecycle points so the frontend can reflect connection state without polling:

| `data.status` | When |
|---|---|
| `connecting` | Before each Loki WebSocket dial attempt or K8s stream open |
| `streaming` | After first successful Loki connection |
| `reconnecting` | When the Loki channel closes and the loop will retry |

Heartbeats changed from SSE comments (`: heartbeat`) to named events (`event: heartbeat\ndata: {}`), which `EventSource` can handle via a dedicated listener.

## Migration

No changes required. The Tail button replaces the previous Live button. No proxy configuration changes are needed beyond ensuring SSE connections are not cut by a write timeout.
