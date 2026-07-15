# Fix: chat Stop/Send button state when switching conversations

## Summary

Switching between an agent's chat conversations showed the wrong composer button:
the Stop/Send state from one conversation carried over to another, or the Stop
button lagged/failed to appear while a conversation was streaming. The button was
also unreliable on the first message of a brand-new chat.

## Design

`use-deployment-chat` is built to switch conversations **in place**: a layout
effect re-seeds the streaming state whenever `activeConversationId` changes, and
the SSE subscription re-scopes to the active conversation. But `ChatWorkspace`
keyed the chat runtime on `${deploymentId}:${conversationId}`, so React remounted
the entire runtime provider on every conversation switch. That defeated the
in-place logic — on a fresh mount `prevConversationRef` always starts `null`, so
the seed-from-cache branch never ran — and forced a from-scratch re-derivation
plus an SSE teardown/reopen on each switch. The result was a window where the
composer showed the wrong Stop/Send button (and general flakiness from the
churn). The same remount fired on the first send of a new chat, when the draft's
`conversationId` (`null`) became a real id, tearing the runtime down mid-stream.

The fix keys the runtime on the **agent** (`deploymentId`) only. Switching
conversations now re-scopes the hook in place, so the button reflects the active
conversation's real streaming state and the live SSE is preserved across the
draft→id transition. Per-conversation scroll and view reset are unaffected —
they're handled by the inner `ThreadPrimitive.Viewport` key
(`conversationId ?? "draft"`), which wraps the message list and composer. The
runtime still remounts when switching agents, which is the boundary that key
now expresses.

A hook test covers the in-place switch: a `rerender` with a new `conversationId`
must flip `isStreaming` to the new conversation's state rather than carry the
previous one's.

**Per-conversation composer drafts.** Because the runtime (and its assistant-ui
composer store) no longer remounts on a conversation switch, an unsent draft
would otherwise persist across conversations and leak into a different thread.
Rather than blank the composer on every switch (which loses the draft), drafts
are now scoped per conversation: a small `ConversationDraftPersistence` node
inside the thread mirrors the live composer text to `sessionStorage`, keyed by
deployment + conversation (`@/lib/chat/chat-draft`), and restores it when the
user returns — including across a page reload within the tab. A sent/cleared
composer removes its slot, so a draft can't resurrect. Deleting a conversation
also drops its draft slot (`clearDraft`), so an unsent draft doesn't outlive the
conversation it belonged to. sessionStorage (not localStorage) keeps drafts
tab-scoped rather than lingering forever; access is SSR/quota-guarded. Covered by
tests: no cross-conversation leak, restore on return, no resurrection after send
or delete, survives remount, and no clobber on an unrelated re-render.

**Stream lifetime scoped to the turn, not the view.** With the runtime now
persisting across conversation switches, the SSE stream was still bound to the
active conversation — switching away closed it, so a turn that finished in the
background was never observed and its cached thread stayed "streaming", leaving a
stale Stop button on return. Streams are now held in a per-conversation map whose
entries live until the turn finishes (`finalizeConversation`) or the hook
unmounts, not across a switch. A turn keeps streaming and finalizes in place
while another conversation is on screen (its partial reply is even up to date on
return), and `finalizeConversation`/`patchAssistantChunk` compare against the
current active conversation (via a ref) so a background finish only updates that
conversation's cached thread. A hook test covers switching away from a streaming
conversation, finishing it, and returning to a cleared Stop button.

**Per-stream stall watchdog.** Because a stream now outlives the active view, a
turn that stalls without ever emitting a finish (a hung or reaped-but-not-closed
sidecar generation) could otherwise pin its `EventSource` open indefinitely — and
the old timeout, which was armed only for the on-screen conversation, both left
the stalled stream in the map (so a resend was short-circuited by the "already
streaming" guard into reusing a dead stream) and never covered background turns.
Each stream now arms its own watchdog when it opens, keyed by conversation and
cleared by `finalizeConversation` on a normal finish. On fire it finalizes the
turn — closing and removing the stream so a resend opens a fresh one — and, only
when that conversation is the one on screen, surfaces the "response timed out"
notice and suppresses the server's lagging streaming snapshot so the composer
unblocks. The send-failure path likewise closes any stream that opened before the
send threw. A hook test covers a stalled turn being reaped and a subsequent
resend opening a new stream.

**History-list "reply in progress" dot cleared on finish.** The conversation
history dropdown renders a dot from each conversation's `assistant_streaming`,
which comes from the list query — a separate cache from the per-conversation
thread. Finishing a turn patched the thread but not the list, so the dot lingered
on conversations that had actually finished. `finalizeConversation` now also
clears that conversation's `assistant_streaming` in the list cache.

## Migration

None. Behavior-only fix to the chat composer.
