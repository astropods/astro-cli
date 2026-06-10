# Deployment chat (platform API)

## Summary

Introduces a **platform deployment chat API** on astro-server so any signed-in client can message agents with durable history, while the messaging sidecar handles only send + SSE. CLI and other clients should use the same REST contract (see `docs/04-guides/deployment-chat.md`). List summaries expose `messaging_web_configured` for chat-eligible deployments.

## Design

**Platform history API:** `GET/PUT/POST /api/v1/deployments/:id/chat/conversations…` — Postgres tables `deployment_chat_conversations` / `deployment_chat_messages`, scoped by deployment + WorkOS user. Not tied to a single client; OpenAPI tag `Chat`.

**Platform transport:** Existing messaging proxy (`POST …/messaging/conversations/…/messages`, SSE stream). Sidecar in-memory history is not source of truth.

**Streaming persistence:** The messaging proxy mirrors assistant SSE into Postgres incrementally (`UpsertAssistantProgress` under a per-conversation advisory lock) so parallel stream consumers cannot append duplicate rows. `GetConversation` dedupes corrupt history (duplicate `seq`, consecutive assistant runs) on read. The proxy persists the user message on send and the assistant message when the stream emits `finish`.

**List eligibility:** `GetMessagingWebConfigured` (messaging sidecar + `http` service) → `messaging_web_configured` on deployment summaries.

**Eligibility:** List filter uses `messaging_web_configured`; send is gated on `GET …/status` → `active` and optional `GET …/runtime` → `messaging_reachable`.

## Migration

Apply `sql/astro-server/schema.sql` (Atlas) before using chat history endpoints. Integrate against `docs/04-guides/deployment-chat.md`; no client-specific APIs beyond the shared REST contract.
