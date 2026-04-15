# Live Log Streaming (Tail Mode)

## Summary

Deployment logs can be streamed live via SSE. The backend opens a log source from `time.Now()` (Loki WebSocket or K8s pod logs), forwards lines as SSE events, and keeps the response open across source reconnections. The browser uses the native `EventSource` API and auto-reconnects if the SSE response itself closes.

## Data Flow

```
Browser (EventSource)
    │
    │  GET /api/v1/deployments/:id/logs/stream
    │  ?workload=&container=&pod=
    ▼
StreamDeploymentLogs (Gin handler)
    │
    ├─ auth + deployment resolution
    ├─ emit: event: ready
    ├─ start heartbeat ticker (5 s)
    │
    ├── [Loki available] ────────────────────────────────────────┐
    │       │                                                     │
    │   TailLogs()                                               │
    │   loki.Client dials WS: /loki/api/v1/tail                 │
    │   query: {namespace="...", pod=~"<workload>-.+",           │
    │            container="..."}                                 │
    │       │                                                     │
    │       ▼                                                     │
    │   chan LogLine ──► select loop                             │
    │       ├── log line  → marshal → SSE message event         │
    │       ├── heartbeat → SSE heartbeat event                 │
    │       ├── chan close → pause 500 ms → re-dial Loki        │
    │       │               emit: status reconnecting            │
    │       └── ctx done  → return                              │
    │                                                             │
    └── [Loki unavailable] ──────────────────────────────────────┘
            │
        resolvePodForStream()
        CoreV1().Pods().GetLogs(Follow=true, SinceTime=now)
            │
        goroutine: bufio.Scanner → parse RFC3339Nano prefix
                                 → send to chan LogLine
            │
        select loop (same as above, no reconnect)
    │
    ▼
SSE event stream (text/event-stream)
    │
    ▼
LogStreamProvider (React context, EventSource listener)
    │
    ├── event: ready      → status: tailing
    ├── event: message    → append to line buffer (cap 5000)
    ├── event: status     → (connecting / streaming / reconnecting)
    ├── event: error      → set error state
    └── onerror           → if hasBeenLive: status: reconnecting
                            else: stream_error
    │
    ▼
LogsTab → LogViewer (render + auto-scroll)
```

## Backend Design

### SSE handler (`StreamDeploymentLogs`)

Sets `Content-Type: text/event-stream`, `X-Accel-Buffering: no`, and disables the write deadline via `http.NewResponseController().SetWriteDeadline(time.Time{})` so the connection is held open indefinitely.

**Event types:**

| Event | When |
|---|---|
| `ready` | After SSE headers flushed; confirms handshake |
| `status` | Backend lifecycle transitions: `connecting`, `streaming`, `reconnecting` |
| `message` | Each log line; JSON `{timestamp, level, message}` with `id: <unix_nano>` |
| `heartbeat` | Every 5 s; prevents proxy timeout |
| `error` | Unrecoverable failure; JSON `{message}` |

### Loki path

`TailLogs()` dials Loki's WebSocket tail endpoint (`/loki/api/v1/tail`) with a LogQL stream selector built from namespace, workload (regex match), and container. It returns a `chan LogLine` and reads frames in a goroutine; a 90 s read deadline causes the WebSocket to close if Loki goes silent.

The handler wraps the channel in a reconnect loop:

- First dial failure → emit `event: error`, return (connection is new, nothing to recover).
- Subsequent dial failures → emit `status: reconnecting`, pause 500 ms, retry (SSE stays open, browser sees no interruption).
- Channel close (normal Loki idle timeout) → same retry path.

### K8s fallback

Used when Loki is unavailable. `resolvePodForStream()` lists pods labeled `app.kubernetes.io/managed-by=astro-server`, filters by workload name prefix and optional container, and prefers Running pods.

Logs are fetched with `Follow=true` and `SinceTime=time.Now()`. A goroutine scans lines and parses the RFC3339Nano timestamp prefix; falls back to `time.Now()` if the prefix is absent or malformed. There is no reconnect loop — when the K8s stream ends the handler returns and the browser EventSource retries.

### Timestamp & level normalization

Both paths produce `loki.LogLine{Timestamp, Level, Line}`. The handler converts these to the wire format via `lokiLineToEntry()`:

```
{
  "timestamp": "<RFC3339Nano UTC>",
  "level":     "<raw level string>",
  "message":   "<line with trailing newlines trimmed>"
}
```

Level normalization (INFO/WARN/ERROR/DEBUG bucketing) happens in the frontend `log-utils` library.

## Frontend (brief)

`LogStreamProvider` holds the `EventSource` ref at the `ActiveDetailView` level so the connection survives tab switches within a deployment view. It tracks a `hasBeenLive` flag to distinguish an initial connection failure (hard error) from a drop after a live session (show "Reconnecting…", let EventSource retry). Lines are buffered up to 5 000 entries.

`LogsTab` owns the `isTailing` toggle and calls `startStream` / `stopStream` accordingly. An auto-disconnect fires after 30 s if the user navigates away while tailing.
