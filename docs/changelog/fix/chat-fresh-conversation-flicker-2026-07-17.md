# Fix chat thread flicker on a fresh conversation's first fast reply

## Summary

Starting a brand-new chat and getting a fast agent reply made the thread
"flicker" — messages appeared, vanished, and reappeared as the history was
hydrated several times in quick succession. It reproduced in prod (real
sidecar write→read lag and network timing widen the race) far more than locally.

## Design

The conversation-history query is intentionally fresh: `staleTime: 0` +
`refetchOnMount: "always"`. On a **new** conversation, the first send is what
creates the conversation id, so the moment it lands the query activates —
mid-turn — and runs a full fetch concurrently with the optimistic user row and
the live SSE chunks. Because a just-created conversation's server snapshot is
briefly inconsistent (empty or user-only), that fetch overwrites the single
cache entry the thread renders from, then the stream rebuilds it, then the
finish invalidation refetches again — each write re-renders the thread.

The existing-conversation send path guards this with `cancelQueries`, but the
create path couldn't: react-query runs an **initial** fetch for the
optimistically-seeded query regardless of `refetchOnMount` (that option only
governs re-fetches of already-fetched queries), so neither `cancelQueries` nor
`refetchOnMount: false` prevents it.

The fix intercepts at the query function instead. While a live SSE stream is
feeding the active conversation, the cache — optimistic user row plus streamed
chunks — is authoritative, so the query serves the cache rather than a network
full-replace:

```ts
queryFn: async () => {
  if (liveStreamRef.current) {
    const cached = queryClient.getQueryData(key);
    if (cached) return cached; // don't clobber the in-flight turn
  }
  // ...normal tail-merge / full fetch
}
```

`liveStreamRef` is the hook's existing `sseActiveRef`, true only while a local
stream is open for the conversation on screen. When the turn finishes the flag
clears and the finish invalidation performs the real fetch, reconciling the
temporary streaming ids with the persisted server ids. This is scoped to the
live-stream window: opening a conversation, reload-mid-turn tail polling, and
all non-streaming refetches are unchanged.

## Migration

None.
