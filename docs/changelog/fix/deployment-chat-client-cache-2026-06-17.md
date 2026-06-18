# Deployment chat client cache (single source of truth)

## Summary

Deployment chat UI had duplicate user bubbles, crashes on open, and broken streaming UX when switching conversations mid-reply. Root cause was dual state: TanStack query cache for persisted history plus a parallel `localMessages` array merged on every render. The client now treats the query cache as the only message source; SSE is transport into that cache, not a second thread.

## Design

**Cache-only thread:** `useDeploymentChat` patches the conversation query entry on send (optimistic user row) and on each SSE chunk (`patchConversationUserMessage`, `patchConversationAssistantChunk`). On stream finish, `assistant_streaming` is cleared in cache and the query is invalidated so server-persisted ids replace temporary streaming ids. `mergeLocalAndServerMessages` is removed.

**In-flight detection:** `serverTurnInFlight` trusts `assistant_streaming: false` from the server even when the tail is a user row (avoids stuck spinners after an aborted turn). Optimistic cache patches set `assistant_streaming: true`. Tail polling uses `useTailPollRef` when a turn is in flight without an active SSE session (e.g. reload mid-reply); polling is disabled while SSE is connected.

**Navigation:** `ChatThread` is keyed by `deploymentId:conversationId` so optimistic state cannot bleed across sidebar entries. Viewport `isStreaming` follows `threadIsRunning` (including the pre-chunk “user at tail” phase) so auto-scroll and the loading indicator work after switching back. `DeploymentChatHistoryScroll` pins the viewport during streaming via `ResizeObserver` as a backup to assistant-ui `autoScroll`.

**Query hygiene:** Conversation queries use `staleTime: 0` and `refetchOnMount: always`. Cache helpers treat `messages: null` from the API as `[]` so `serverTurnInFlight` and tail merge do not throw.

## Migration

Client-only. Deploy astro-client with no server or agent changes.
