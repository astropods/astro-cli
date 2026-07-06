# Fix: chat "stop generating" actually stops the turn

## Summary

The chat composer's "stop generating" button previously only closed the
browser's SSE connection. The agent kept generating, its full reply still landed
in history (and reappeared on reload), and the conversation stayed stuck in the
streaming state. Stopping now ends the turn everywhere: the stream stops, the
partial is what's kept, and — on a cooperating runtime — the model generation is
actually cancelled.

## Design

The stop spans four layers, each with a distinct responsibility.

### Messaging sidecar (transport + turn control)

New endpoint `POST /api/conversations/{id}/cancel`. On stop the web adapter:

- marks the turn stopped and drops the agent's subsequent content chunks until
  the agent's next `START` chunk — so a non-cooperating agent's trailing or
  complete output can't be delivered as the *next* turn's response;
- emits a terminal `finish` event and then closes the conversation's SSE
  connections, so the astro-server chat-store persister unwinds instead of
  lingering and re-marking the turn active;
- forwards a `StreamControl{STOP}` to the agent over the gRPC stream.

### Client

Adds a `cancelMessagingStream` call fired from the composer's stop handler, and
suppresses turn-reopen for a conversation the user explicitly stopped, so a
lagging "still streaming" history snapshot can't resurrect the cancelled turn.
Suppression clears on the next send or a conversation switch.

### astro-server (in-transit proxy + history)

- A proxied `/cancel` is treated as a turn terminator: it clears the
  `assistant_stream_active` marker immediately, so a history refetch reads "not
  in flight."
- History hydration treats a stopped turn's Langfuse control-marker output
  (e.g. `{"status":"aborted"}`) as empty, so the persisted partial wins instead
  of the marker being shown as the assistant message on reload.

### Agent SDK / adapters (actual cancellation)

`StreamOptions` gains an `AbortSignal`. The messaging bridge maps each in-flight
turn to an `AbortController`, forwards the signal into the agent's stream, and
aborts it when the STOP feedback arrives. The Mastra adapter passes it as
`abortSignal` to `agent.stream()`, so Mastra cancels the model request and
telemetry records only the partial. Runtimes that ignore the signal degrade to
user-visible stop only (the model finishes in the background).

## Limitations

- Cancelling generation depends on the agent's runtime honoring the forwarded
  abort signal. Mastra does; a hand-rolled runtime that ignores it will run to
  completion.
- A stopped turn's prompt can still influence the next turn if it remains in the
  agent framework's own conversation memory. Cleaning up interrupted turns is an
  agent/framework concern (memory semantics), not something the platform stop
  imposes.

## Migration

No action required. No schema or client-contract changes; the `/messaging/*` and
`/chat/*` shapes are unchanged and the stop endpoint is additive. The messaging
submodule is bumped for the sidecar endpoint, and the `@astropods` adapter
packages carry the abort wiring (agents pick it up on their next adapter
version).
