# Chat "stop generating" is handled by the messaging sidecar

## Summary

The chat composer's "stop generating" action now actually ends the turn.
Previously it only closed the browser's SSE connection: the agent kept running,
its reply was still persisted on completion, and the conversation stayed in the
"streaming" state. Stopping did nothing server-side.

Because the chat API is moving into the messaging container, the fix lands in
the sidecar (its future owner). A new stop endpoint on the sidecar ends the
turn deterministically for every agent, with no per-agent code. astro-server
forwards it through the existing messaging passthrough proxy, and the client
calls it when the user hits stop.

## Design

- New sidecar endpoint `POST /api/conversations/{id}/cancel`. On stop the web
  adapter marks the turn stopped, persists the partial reply the user had seen,
  flips the conversation out of the streaming state, sends a terminal `finish`
  event to any live SSE stream, and forwards a `StreamControl{STOP}` signal to
  the agent over the gRPC stream.

- Late output is dropped, not raced. A per-turn tracker records the streamed
  partial and, once a turn is stopped, discards the agent's remaining chunks so
  a non-cooperating agent's late/full reply can't overwrite the partial or
  resurrect the stream. Tracker state resets when the next user message starts a
  new turn.

- The turn becomes terminal in the store. `FinalizeStopped` appends an assistant
  row carrying the partial when the latest message is still the user's, so the
  derived `assistant_streaming` flag resolves to false. It never shrinks a reply
  that already finished (or was progressively persisted).

- astro-server is unchanged. The messaging proxy is a generic passthrough, so
  the cancel request reaches the sidecar without new server routing. The client
  gains a `cancelMessagingStream` call, fired best-effort from the composer's
  stop handler before local teardown.

- The web adapter now retains the feedback handler (previously a no-op), which
  is what carries the stop signal to the agent.

## Limitations

- Stop is user-visible only. The browser turn ends immediately and the partial
  is persisted, but the agent's model call runs inside the agent process — the
  sidecar cannot halt it. A non-cooperating agent keeps running and continues to
  spend tokens and emit telemetry (Langfuse) until it finishes on its own; the
  sidecar simply discards that late output.

- Truly halting generation (stopping token spend) is a follow-up (L2): the
  messaging SDKs will own a managed handler that cancels a per-turn signal when
  `StreamControl{STOP}` arrives, so agents get real abort without writing abort
  logic. This changelog covers only the sidecar-owned stop.

- If the live SSE drops and the client falls back to polling, the stopped
  partial isn't visible until the stop completes.

## Migration

No action required. No schema, infrastructure, or client contract changes — the
`/chat/*` and `/messaging/*` request/response shapes are unchanged; the stop
endpoint is additive.
