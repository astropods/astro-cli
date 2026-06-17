# Deployment chat history (Langfuse + Postgres)

## Summary

Deployment chat now survives navigation, reload, and cross-device use. Conversation **metadata** (sidebar title, recency, soft-delete) lives in astro Postgres keyed by the opaque WorkOS user id — no message bodies or PII in that table. **Message content** is hydrated on read from Langfuse traces (`session_id` = `conversation_id`) when traces are complete; otherwise from Postgres rows written by the messaging proxy. This restores durable history after the June 2026 removal of chat Postgres persistence, without making platform RDS the long-term content store where Langfuse is configured.

## Design

**Split storage:** `deployment_chat_conversations` holds per-user metadata only. `deployment_chat_messages` holds message bodies as a durable fallback when Langfuse traces are absent or lag behind proxy persistence (typical in local dev without OTEL export, or mid-turn before traces flush). Langfuse remains the preferred source when its thread is at least as long as Postgres.

**Read path:** `GET …/chat/conversations/:id` verifies ownership via the metadata row, then hydrates oldest-first from both Langfuse and Postgres and returns the **superset** (Postgres wins when it has more messages — e.g. traces not yet landed). `assistant_stream_active_at` signals an in-flight assistant reply.

**Write path — messaging proxy only:** User sends and assistant SSE are the sole persistence path. The proxy tees `POST …/messaging/conversations/:id/messages` into `AppendUserMessage` (with per-conversation ownership check — a UUID owned by another user returns 409) and mirrors stream chunks into `UpsertAssistantProgress` under an advisory lock. Client upserts metadata (title on first turn, recency touch) via `PUT …/chat/conversations/:id`. The legacy `PUT`/`POST …/chat/conversations/:id/messages` routes are removed; they were never called by astro-client and duplicated the proxy path.

**Client:** Sidebar is server-backed (list, rename, delete). Opening a conversation refetches history via TanStack Query (`staleTime: 0`, `refetchOnMount: always`) so a stale empty cache cannot mask persisted threads. Live turns append locally and dedupe against the server snapshot by tail-anchored `(role, content)` matching.

**Langfuse client:** `GetSessionTraces` filters by deployment tag, `userId`, and `sessionId` for conversation-scoped hydration.

## Migration

Apply `sql/astro-server/schema.sql` (Atlas) before deploying — adds `deployment_chat_conversations`, `deployment_chat_messages`, and `assistant_stream_active_at`. No agent changes required. Ensure `ASTRO_REDACT_PROMPTS` is off on the collector where Langfuse should retain message bodies. External clients that called the removed chat message write routes must use the messaging proxy instead (`POST …/messaging/conversations/:id/messages` + SSE stream).
