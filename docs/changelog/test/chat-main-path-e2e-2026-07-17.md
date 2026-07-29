# e2e coverage for the chat main path

## Summary

The full-page chat feature had e2e coverage only for its window controls
(auto-focus, header title, open-in-new-tab) — the core interaction, sending a
message and receiving a streamed reply, was untested end to end. This adds a
Playwright spec that exercises that main path against the mock backend.

## Design

The send path is plain HTTP: a `POST .../messaging/conversations/{id}/messages`
followed by an SSE `GET .../messaging/conversations/{id}/stream` (native
`EventSource`) that carries the assistant's tokens. There is no WebSocket, so
the existing HTTP mock backend can serve the whole exchange — it just lacked the
routes.

The mock backend (`e2e/mock-backend.ts`) gains a small stateful chat store keyed
by conversation id, seeded with the existing demo thread:

- `POST .../messages` appends the user turn plus a canned assistant reply.
- `GET .../stream` replays that reply as `chunk` events followed by `finish`,
  matching the sidecar wire format in `lib/messaging/transport.ts`.
- The chat history read serves from the same store.

Keeping the streamed reply and the persisted thread in sync is what makes the
test stable: when a turn finishes, the client invalidates and refetches history
(`finalizeConversation`), so the refetch must return the same content that was
streamed or the reply would flicker away. The store is reset between tests via
the existing `/test/reset` hook.

The spec (`e2e/chat-send-message.spec.ts`) covers the happy path — type, send,
see the optimistic user bubble, watch the assistant reply stream in, and confirm
the composer re-arms afterward — plus a follow-up turn to verify the thread
accumulates across turns. Composer readiness is forced the same way the existing
chat spec does it, by overriding the deployment `runtime` response to
`messaging_reachable: true`.

## Migration

None. Test-only change.
