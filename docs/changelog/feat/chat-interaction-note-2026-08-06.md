# Resolved-interaction rows in deployment chat

## Summary

When an agent asks the user something inline (a form, a tool approval, or a
free-text reply), the chat now records how that interaction resolved and renders
the agent's follow-up as its own message rather than merging into the text that
preceded the question. A submit/decline shows as a muted "note" line; a "write
your own reply" shows as a normal user message. This is the client half of the
sidecar's interaction turn lifecycle.

## Design

The sidecar injects the resolution as a thread row the client didn't send,
delivered over one `injected` SSE event carrying the row's role. The transport
surfaces it through an `onInjected(id, role, content)` callback, which appends the
row to the TanStack cache: role `note` for a submit/decline record ("Answered · …",
"Approved"/"Denied"), role `user` for a "write your own reply" (the user's prose).

The row doubles as a rendering boundary. Because it is a non-assistant tail, the
streaming-assistant pointer resolves to null, so the agent's continuation opens a
fresh bubble instead of extending the pre-interaction reply. The turn stays in
flight across it: `threadIsRunning` treats any non-assistant tail (a sent user
message, or an injected row awaiting the continuation) as running, keeping the
Stop button and loading indicator up so a message typed in that window isn't
dropped.

A `note` row maps to an assistant-ui `system` message with exactly one text part
(the runtime rejects any other shape) and renders as a centered muted aside via a
`NoteMessage` component — no bubble, no avatar; a `user` row renders as an ordinary
user bubble. The append is idempotent by id so a redelivered event after an
`EventSource` reconnect can't double the row, and the streaming-pointer reset is
scoped to the active conversation so an injected row for a background conversation
can't disturb the on-screen view.

## Migration

None. The `note` role and the `injected` event are additive; a deployment whose
agent never emits an interaction sees no change.
