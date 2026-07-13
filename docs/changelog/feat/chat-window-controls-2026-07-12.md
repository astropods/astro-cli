# feat: chat window controls and auto-focus

## Summary

Small in-app chat quality-of-life gaps from the tracking issue (#1370, sub-issue #1355): the composer did not take focus on launch (an extra click before every message), there was no way to pop a chat into its own tab for multi-window work, and the renamed conversation title was not surfaced anywhere in the chat view. Closes #1355.

## Design

All client-only, layered onto the existing assistant-ui chat surface:

- **Auto-focus.** The composer focuses its input on mount and whenever the agent transitions to ready, driven by the same readiness gate that disables the composer while the agent is starting/paused/unreachable. Focus is skipped while disabled or while dictation has replaced the input.
- **Open in new tab.** The chat header carries an affordance that deep-links to the current agent + active conversation (`/chat/:id?conversation=:cid`) with `target="_blank"`, so a chat can be detached into its own window.
- **Conversation title.** The header surfaces the active conversation's title (server-derived, from the conversation list) between the agent switcher and the controls, truncating when long.

Esc-to-stop and Enter-to-send were already provided by the assistant-ui composer, verified by a regression test rather than re-implemented (a duplicate handler double-fired the cancel). Per-message timestamps were intentionally deferred: the messaging sidecar's message record carries no timestamp, so any client-side time would reflect load time and mislead on reloaded history. Surfacing accurate times needs a `created_at` on the sidecar message record and is left as a backend follow-up.

## Migration

None. Additive chat UI only.
