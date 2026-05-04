# Build heartbeat for long Docker builds

## Summary

Long `ast push` Docker builds went silent for minutes at a time. BuildKit's trace stream prints each vertex name once and emits nothing further until the next vertex starts, so a step like `vite build` transforming several thousand modules under emulation (Apple Silicon → linux/amd64) is indistinguishable from a wedged build for ~7 minutes.

## Design

`streamBuildOutput` now tracks each BuildKit vertex by digest in a `vertexState` map (`name`, `started`, `completed`). The trace handler emits:

- A cyan vertex-name line on first sighting (unchanged).
- A dim `✓ <name> (<elapsed>)` completion line — gated to 5s+ steps by default so fast `COPY`/`FROM` lines don't drown out the build script output the user actually cares about. `--verbose` flips the gate so every step gets timed.
- Existing dimmed log lines, optionally prefixed with `[<truncated vertex name>]` in verbose mode so parallel stages can be disambiguated.

Idle detection is a separate `startHeartbeat` goroutine sharing a mutex with the trace handler. Every inbound message stamps an `activity` timestamp; the goroutine ticks at 15s (5s in `--verbose`) and, only if the silence exceeds one full interval, prints `… still running <vertex> (<elapsed>), ...` listing every in-flight vertex. `defer stopHeartbeat()` keeps the goroutine bounded to the stream lifetime.

`formatElapsed` renders `12.3s` under a minute and `1m 04s` otherwise; `truncateVertexName` clips long `RUN --mount=...` names so heartbeat lines stay readable.

## Migration

None.
