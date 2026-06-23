# Chat agent switcher (P0.1)

## Summary

The web chat page used a conversation-sidebar model: one agent selected via a top-bar menu, threads listed in a left column. v1 of the new IA replaces that with a single **agent switcher** in the thread header — the same dropdown used on the agent detail page (`AgentDeploymentMenu`) — plus a conversation **History** dropdown. No left rail, no account switcher.

## Design

**Cross-account**

- The page lists every chat-eligible agent the user can reach, regardless of org/account, so there is no account scope switcher. `useChatAgents` enumerates the user's accounts from the (already membership-scoped) deployments summary, fans out per-account deployment reads, and keeps the ones with web messaging. Each entry carries its own account so identity/avatars resolve per row.

**Frontend**

- `ChatThreadHeader` — left: `AgentDeploymentMenu` (`variant="detail"`, the exact agent-detail switcher) filtered to chat-eligible deployments, navigating to `/chat/:id`; right: New chat icon button (`SquarePen`) plus `ConversationHistoryDropdown` (History + count).
- `ChatWorkspace` — full-width conversation pane (thread header + thread). The two-column rail layout is gone; the conversation fills the viewport directly under the app header.
- TanStack Query: `useChatAgents` (summary + per-account deployment fan-out). Conversation list summaries still carry `assistant_streaming` for the per-conversation streaming dot.

**Backend (why this frontend branch touches astro-server)**

- The new History dropdown shows a per-conversation "reply in progress" dot, which needs to know — for *every* conversation in the list — whether an assistant turn is currently streaming.
- The single-conversation `GET` already returned `assistant_streaming`, but the conversation **list** endpoint (`ListDeploymentChatConversations`) did not: it only returned `conversation_id` / `title` / `updated_at`. The list is the only data the History dropdown loads, so without this the dot could never render.
- Change is additive and backward-compatible: `ListByUser` now also selects/scans the existing `assistant_stream_active_at` column, a new `AssistantStreamActiveFrom` helper reuses the existing `streamWriteBlocked` active-window logic, and the list summary response gains `assistant_streaming` (`omitempty`). No new column, no migration, no change to write/streaming paths.

**Removed** (rail-era scaffolding, never shipped)

- `ChatAgentRail`, the recency `session-groups` helper, and the `GET /api/v1/accounts/:account/chat/agents` activity endpoint (handler + `chatstore.ListAgentActivity`). Agent recency/unread will return with pins.

**Out of scope (follow-up PRs)**

- Pinned agents
- `last_read_at` / server-authoritative unread badges
- Load-older button polish

## Migration

No schema migration — uses existing `deployment_chat_conversations` columns. Deploy astro-server before or with astro-client.
