## Summary

On a brand-new chat, the very first send could briefly show the agent's loading
dot and then drop it — the streamed reply never rendered and the conversation
was only recoverable by reloading the page. It showed up in production but not
preview. This makes the first turn survive the race that caused it.

## Design

Sending the first message optimistically marks the turn in flight and opens an
SSE stream, while the conversation's history is fetched immediately. On a new
conversation that history GET can resolve before the server has registered the
assistant turn — Langfuse write→read propagation and first-token latency leave a
short window where `assistant_streaming` is false. The client treated that early
"not in flight" snapshot as authoritative, tore down the live stream, and lost
the in-flight reply until a reload re-fetched the (by then persisted) turn. The
window is timing-dependent, which is why slower production agents/propagation hit
it while preview usually did not — the client and server code paths were
identical across the two.

The fix makes a just-sent turn authoritative until its stream actually ends:

- A new `deriveTurnInFlight` helper treats an active local turn (we sent and the
  SSE is still open) as in flight even when the server snapshot reports
  otherwise. The turn ends only via the SSE finish/error path or the existing
  in-flight timeout, after which the server snapshot is trusted again.
- The effect that ends a turn from a server snapshot is gated on no local SSE
  being open, so an early snapshot can't close a live stream.

Defense in depth on the server: the messaging proxy now marks the assistant
turn active the moment the user message is accepted (not only when the assistant
SSE connects), so the history GET in that gap reports the turn as in flight. The
marker is time-windowed and auto-expires if no reply follows.

## Migration

No action required.
