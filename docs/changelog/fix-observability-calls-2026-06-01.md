# Fix: Trace detail 502 for agents with large tool outputs

## Summary

The trace detail panel was timing out (502) for agents that produce large tool call outputs — GitHub PR diffs, Confluence pages, project status reports — because Langfuse was fetching every observation's full `input`/`output`/`metadata` payload in a single unbound ClickHouse query. A trace with 20–30 tool call observations each carrying 30–60KB of content could result in over 1MB of data Langfuse had to load before sending a single byte of response, consistently exceeding the 15s HTTP client timeout.

## Design

**Two parallel Langfuse requests per trace detail.** The `GET /api/public/traces/{id}` endpoint accepts a `fields` query parameter that controls which ClickHouse columns Langfuse fetches. `GetTrace` now fires two requests concurrently and merges the results before returning:

- `fields=core,io,scores,metrics` — fetches the root trace's `input`/`output`/`metadata` plus scores. No observation join; fast regardless of observation count.
- `fields=core,observations` — fetches the observation tree (structural fields only: id, parent, timing, type, model, cost, tokens). No I/O columns; fast regardless of payload size.

The `io` field group cannot be scoped to the trace root only — if included alongside `observations`, Langfuse fetches I/O for every embedded observation in the same ClickHouse scan. Splitting the requests is the only way to get trace-level I/O without triggering the expensive observation I/O scan.

**Observation detail on demand.** A new server endpoint (`GET /api/v1/deployments/:id/observability/observations/:observationId`) fetches a single observation's full content via Langfuse's `GET /api/public/observations/{id}`. This is a primary-key lookup — fast regardless of how large the payload is. The frontend calls this endpoint when a node is selected in the trace tree.

**Frontend lazy-load.** `ObservationDetail` now fetches its own I/O data via `useObservabilityObservationDetail`. The tree renders immediately with structural fields; selecting a node fires a single-observation fetch and shows a spinner while it loads. `TraceObservation` already typed all fields as optional so no type changes were needed.

## Migration

No action required.
