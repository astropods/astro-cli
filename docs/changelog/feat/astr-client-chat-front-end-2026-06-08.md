# Deployment chat (astro-client UI)

## Summary

Adds the first **web UI** for platform deployment chat in astro-client (`/chat`). Uses the server history API and messaging proxy from the backend branch; durable persistence is server-authoritative (no client-side message writes on stream finish).

## Design

**Routes:** `/chat` and `/chat/:deploymentId` with optional `?conversation=` deep link.

**Data:** TanStack Query hooks in `api/queries/chat.ts` for conversations and history. Turn lifecycle is server-authoritative: `GET /chat/conversations/:id` reports `assistant_streaming` (plus a trailing user message meaning "awaiting reply"), and the client derives all in-flight state from that response — no content-stability heuristics. While a turn is in flight the conversation query polls via `refetchInterval` using `?limit=N` tail fetches merged into the cached thread (not a full-thread download each tick). Active sends attach SSE so the proxy begins consuming promptly; the server also persists in a detached context after navigation away, so recovery is tail polling, not a new stream consumer. SSE `finish` triggers a full refetch that ends the turn.

**Storage (schema):** conversations store a denormalized `agent_name` snapshot; `deployment_id` is nullable with `ON DELETE SET NULL` so history survives deployment removal. The REST API remains deployment-scoped for now.

**Scroll:** history load and live streaming follow the viewport only while the user is at bottom; no forced scroll during read-up.

**Turn integrity (server):** the `assistant_stream_active_at` marker is per assistant turn (set on first chunk, cleared on `finish`/`error`) rather than per SSE connection, and a user send that cannot be persisted (active stream, message limit, id conflict) is rejected with 409 instead of being forwarded upstream while silently missing from history — preventing cross-turn assistant-row merges.

**UI:** assistant-ui thread/composer (`DeploymentChatRuntimeProvider`, `DeploymentChatThreadView`), Streamdown for streaming markdown (math/mermaid plugins + selective controls ported from `astropods/playground#28`), conversation sidebar on desktop and full-width list/thread swap on mobile below `md`.

**Eligibility:** Deployment list filtered by `messaging_web_configured`; send gated on messaging `active` status and runtime reachability.

## Migration

Requires the backend chat API and schema from `feat/astro-client-chat-backend`. No additional server changes. Local web chat uses the K8s messaging proxy (not `MESSAGING_URL_OVERRIDE` or NodePort Launch).
