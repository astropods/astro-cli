# Restore trace cost under Langfuse v4

## Summary

Trace cost read as zero across the traces list and the trace detail panel. Under v4 a listing returns each trace's root span, and cost sits on the child `GENERATION` spans, so a root's own cost is zero. Nothing summed it.

## Design

**The listing totals cost with one aggregate query.** `getTraces` follows its `/v2/observations` page with a single `/v2/metrics` call grouped by `traceId`, filtered to the trace IDs on that page plus the deployment tag, and fills each row's total. One call per page, not per trace.

Langfuse aggregates the same way internally. Its `events_traces` view groups the events table by `trace_id` and sums the spans:

```sql
totalCost: sumIf(toNullable(events_traces.total_cost), events_traces.parent_span_id != '')
```

That view is declared for v2 but withheld from the public API, whose own comment reads "Public v2 API views - excludes traces. Internal dashboard queries still support the events-backed v2 traces declaration for legacy widget parity." So the same aggregate has to be requested through `view:"observations"` grouped by `traceId`. `traceId` is a high-cardinality dimension, which means v4 requires `orderBy` and `config.row_limit`; a page never approaches the server's 1000 ceiling.

A failed aggregate fails the request rather than defaulting to zero. Zero is indistinguishable from a free trace on the page, which is the bug this fixes.

**Every span counts toward the total, including the root.** Langfuse's view excludes it with `parent_span_id != ''`, which reports nothing for a single-span trace whose only span is the generation that cost money. Including the root is free when it is a plain span, because its cost is then zero. The listing and the detail aggregate identically, so a trace's cost matches in both places.

**The detail panel prefers the detail response for cost.** `TraceDetailPanel` reads header tiles from the list entry so it can render before the detail fetch resolves. Cost is the exception, because the list value is a root span's zero, and `??` treats zero as present rather than falling through. The panel now takes the detail's total and keeps the list value only as a placeholder.

## Migration

None. No configuration changes and no API shape changes.

The traces list issues one additional Langfuse request per page.
