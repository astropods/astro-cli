# Fix Langfuse v4 trace reads

## Summary

Every trace detail request returned 404 once preview started reading the Langfuse v4 API, and the trace list showed child spans as though each were its own trace. Trace timestamps rendered as an invalid date and trace costs read as zero. Three defects in `v4Reader` caused this, all traceable to one habit: sending a v3-shaped request to a v4 endpoint and trusting that a familiar name still behaves the same way.

## Design

**Root detection moves from a query param into the filter array.** `/v2/observations` silently ignores a dedicated `isRootObservation` query param, so the listing returned every span and each child appeared to be its own trace. The same silent-ignore applies to `tags`, `sessionId`, and `orderBy`; the endpoint only honors the JSON `filter=` array. Root selection is now a filter entry:

```json
{"type": "boolean", "column": "isRootObservation", "operator": "=", "value": true}
```

Langfuse resolves that column to `(parent_span_id = '' OR is_app_root)`, so it matches a physical root and also a root whose parent span was never ingested, which happens when the root lives in another service. A null filter on `parentObservationId` covers only the first case, so the flag is the better predicate. The `/v2/metrics` count query already filtered this way and is unchanged.

**Traces are keyed by trace ID, not by their root span's ID.** This follows Langfuse's guidance for the v4 data model, where a trace is every row sharing a `traceId` and the API returns observation rows rather than trace objects. Trace detail fetches the whole tree in one call, keyed by `traceId`, and reads trace-level fields from the root row, one round trip fewer than resolving the root separately.

**Reads request the field groups their response actually needs.** Field groups are additive and default to omitting most fields, so a missing group silently yields zero values:

| Group | Supplies |
|---|---|
| `trace_context` | `tags`, `release`, `traceName` |
| `metadata` | `metadata` |
| `model` | `model`, `modelParameters` |
| `usage` | `totalCost`, `inputUsage`, `outputUsage`, `totalUsage` |
| `metrics` | `latency` |

`trace_context` is the one that caused the 404. `GetLangfuseTraceDetail` rejects a trace whose tags omit the caller's deployment, so a response with no tags failed that check for every trace. `CreatedAt` was empty for the same reason and rendered as an invalid date; it now comes from the root span's start time.

**Trace cost sums across spans.** Cost and tokens sit on child `GENERATION` spans and do not roll up onto the root, so reading `totalCost` from the root reports zero for every trace. Measured on four real traces: zero on the root against roughly $0.0128 summed. `getTraceDetail` totals the tree.

Field names come from Langfuse's `ObservationSchema`, not from inference. A test decodes a literal response body so a wrong key fails the build instead of silently decoding to a zero value.

## Migration

None. No configuration or API changes, and the response shapes callers see are unchanged.

One behavior change is worth knowing: trace IDs in `/observability/traces` responses are now Langfuse trace IDs rather than root span IDs. Links captured from the previous build will not resolve.

The trace list still reports zero cost per trace, because it fetches only root spans and a correct total needs the whole tree. Detail views report cost correctly. Closing that gap needs an extra grouped `/v2/metrics` query per page.
