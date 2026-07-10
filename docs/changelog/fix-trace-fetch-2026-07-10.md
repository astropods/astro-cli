## Summary

The deployment traces view failed to load whenever it asked for more than 100 traces. The Monitor tab requested a 500-trace window in a single call, but the upstream Langfuse traces API rejects any request with `limit > 100`, so the server returned a 502 and no traces rendered.

## Design

The traces endpoint already exposes `limit`/`offset` pagination but never enforced Langfuse's per-request ceiling. The server now clamps `limit` to 100 (the documented and upstream maximum) so a single request can never exceed what Langfuse accepts. Wider windows are assembled by paging through `offset`, which the endpoint already supported.

The Monitor tab consumes that pagination instead of over-fetching. A dedicated infinite query pages the traces endpoint at 100 per request, deriving the next offset from the returned `total`, and flattens the pages into the loaded window. The single-page traces hook is unchanged, so callers that only need a small slice are unaffected.

The existing "Show more" control in the traces table drives pagination rather than introducing a separate button: collapsed it reveals the rest of the loaded page, and once expanded it offers "Load more" to pull the next page from the server (alongside "Show less"). This keeps one affordance for both local expansion and server fetch.

## Migration

None. The change is transparent to users — the traces view loads an initial page and fetches more on demand. Trace search and status filters remain client-side over the loaded window, matching prior behavior.
