# Langfuse Dataset Sync Load Reduction

## Summary

Large dataset syncs could overload Langfuse by fetching more trace data than the dataset writer needed and by making an extra trace-detail request for every item written. This change reduces Langfuse read pressure while preserving the dataset input, expected output, metadata, and source trace linkage needed for evaluation datasets.

## Design

Dataset sync now uses the Langfuse trace `fields` parameter with `core,io` when reading trace pages. That keeps the trace identity and I/O payloads Astro needs while avoiding scores, observations, and metrics that are not used when creating dataset items.

Dataset items no longer store `sourceObservationID`. The source trace ID remains on every item, so items can still be tied back to the originating trace without a per-trace observation lookup. This removes one Langfuse read per written trace and avoids hydrating observation trees just to find a root observation.

Trace pages are now requested with `orderBy=timestamp.asc`. The Langfuse default order is unspecified, so offset-based paging across multiple page requests is only stable when an explicit sort is supplied. Ascending timestamp on a frozen `[fromTimestamp, toTimestamp]` window guarantees that a trace seen on page N cannot shift onto a later page between requests and be skipped.

## Migration

No user action is required. Existing Langfuse dataset items that already have `sourceObservationID` keep it. Newly synced items rely on `sourceTraceID` for trace linkage.
