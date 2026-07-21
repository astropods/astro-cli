# Fix: chat turn stuck on the loading avatar after a reply already completed

## Summary

A chat turn could stay pinned on the animated "thinking" avatar even though the
agent's reply had finished and persisted — the response was visible in the trace
and appeared on a page refresh, but the live view never left its loading state.

Root cause is a structural gap, not a single bad timeout: the chat SSE was
**not resumable**. The sidecar fanned each event out only to the connections
present at that instant and buffered nothing, and every subscription began a
fresh stream that replayed no prior state. That bakes in a false invariant — "a
subscribed client receives every event exactly once" — but a long-lived
connection *will* break (a proxy/LB hop, a network blip, an `EventSource`
reconnect, an app resubscribe). Whenever the terminal `finish` was broadcast
while the client was between connections, it hit zero subscribers and was lost
forever; the client then sat in its loading state until a 3-minute watchdog.
This is most likely on a frontier-model agent, whose turns are long enough to
span a reconnect. (#1491 patched an earlier symptom of the same gap — an early
"not in flight" snapshot tearing the turn down — but left the lost-terminal-event
case.)

## Design

Make the stream resumable so a broken connection is a performance detail, not a
correctness problem:

- **Monotonic event ids + per-conversation resume buffer.** The sidecar now tags
  every broadcast event with a per-conversation sequence number (emitted as the
  SSE `id:`) and retains a bounded, oldest-first ring of recent events
  (per-conversation cap, plus an LRU cap on total conversations). The buffer is
  independent of connection lifetime, so an event survives a window with zero
  connections.

- **Honor `Last-Event-ID` on (re)subscribe.** A browser `EventSource` replays its
  last-seen id on reconnect automatically. The stream now reads it and replays
  exactly the events with a higher sequence — the missed chunks *and* the
  terminal `finish` — before resuming live output. Registration and the replay
  snapshot are taken under the same lock as broadcast, so an event is never both
  replayed and delivered live. The astro-server messaging proxy was updated to
  forward the `Last-Event-ID` header to the sidecar (it previously dropped it).

- **Terminal-state replay for a fresh subscribe.** A subscribe with no
  `Last-Event-ID` (the reply finished before the client's first subscription)
  can't resume from a cursor, so the stream falls back to emitting a `finish`
  when the store shows the turn already terminal. A safety net covers a cursor
  that predates the retained buffer.

A fresh subscribe deliberately does **not** replay buffered deltas (the client
reconstructs the reply by appending, so re-sending would double it) — resumption
is opt-in via the cursor. No client change is required for resumption: the
browser already sends `Last-Event-ID` and reconnects on its own; the replayed
events arrive as ordinary chunk/finish events.

This supersedes narrower point-fixes for this bug class: replay-on-subscribe is
just the terminal-only special case of general resumption.

- **Fresh subscribe settles instead of guessing.** The store snapshot alone
  can't tell a finished turn from a new one whose user row hasn't persisted yet,
  and a just-finished turn may have broadcast its `finish` to zero connections
  while its streaming flag lingered. Rather than decide synchronously — which
  misfired both ways (a spurious `finish` on a follow-up turn racing its send, a
  missed `finish` mid-teardown) — the registered connection observes the wire for
  a bounded window: a live chunk means the turn is live, a live finish ends it,
  and only silence falls back to the store-derived terminal replay.

- **New conversations are created before the stream subscribes.** "New chat" used
  to navigate to a client-generated conversation id, so the first send lazily
  created the row while the SSE stream subscribed to it in parallel — the stream
  could reach the sidecar first and get a 404 for the not-yet-created
  conversation, hanging the turn until a reload. New chat now opens a blank chat
  with no id; the row is created server-side on first send
  (`createMessagingConversation`) before the stream opens, and the URL is then
  updated to the real id. The CLI's bundled chat SPA reuses the same page, so it
  inherits this fix. Two follow-ons keep that blank→id transition invisible: the
  thread viewport keys on the hook's stable active conversation id (not the
  lagging URL param) so the id landing mid-reply no longer remounts and flickers
  the thread, and the auto-select-latest effect runs once per agent so a
  deliberate "New chat" isn't bounced back to the most recent thread.

## Migration

None. No config, API, or schema changes; the SSE wire format is unchanged apart
from now always carrying an `id:` field.
