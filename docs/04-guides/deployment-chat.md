# Deployment chat (platform API)

Canonical persistence and transport for **any** Astro client where a signed-in user messages a deployed agent (web, CLI, mobile, etc.). History lives on **astro-server**; the deployment **messaging sidecar** is transport only (send + SSE stream). Sidecar in-memory history is not durable and must not be used as source of truth.

## Responsibilities

| Layer | Owns |
|-------|------|
| `GET/PUT/POST /api/v1/deployments/:id/chat/...` | Conversation list, titles, message history (Postgres) |
| `POST/GET /api/v1/deployments/:id/messaging/...` | Proxy to sidecar: create conversation (optional), send message, SSE stream |

Clients **read** history from the chat API. **Writes** to history during a turn are performed by astro-server inside the messaging proxy (user message on send, assistant message during SSE). Clients must not duplicate that persistence.

## Auth and scope

- Same session as the rest of the Astro API (WorkOS user in middleware).
- Deployment ownership: caller must be a member of the deployment’s account (`resolveDeployment`).
- History is keyed by **`deployment_id` + `user_id`**. Two users on the same account do not share threads.

## Conversation identity

- Clients choose a **UUID v4** `conversation_id` before the first message (recommended).
- Use that **same id** for:
  - `PUT /chat/conversations/:conversationId` (register title)
  - `POST /messaging/conversations/:conversationId/messages` (send)
  - `GET /messaging/conversations/:conversationId/stream` (SSE)
- If the client omits a pre-created id, the sidecar may assign one on first send; register that id via `PUT /chat/conversations/:conversationId` so list/title APIs stay aligned.

## Chat API (history)

Base: `/api/v1/deployments/:deploymentId/chat`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/conversations` | List summaries for this user + deployment (most recent 200) |
| GET | `/conversations/:conversationId` | Thread (`messages[]`) plus `assistant_streaming` — true while the proxy is persisting an assistant reply. Omit `limit` for full history. Optional `?limit=N` returns the tail (or older page with `?before_seq=S`); response includes `has_more` and `oldest_seq` when paginated. |
| PUT | `/conversations/:conversationId` | Create/update title (`{ "title": "..." }`); returns 409 if the id is owned by another user/deployment |
| POST | `/conversations/:conversationId/messages` | Append one message (`id`, `role`, `content`) — optional; prefer proxy persistence on send |
| PUT | `/conversations/:conversationId/messages` | Replace full thread — legacy/bulk sync; returns **409** while the messaging proxy is persisting an assistant SSE stream |

**Typical client flow**

1. `PUT` conversation (title, optional empty thread).
2. On user send: messaging `POST` message (server persists user row before upstream) → messaging SSE for live chunks.
3. Poll or refetch `GET /conversations/:conversationId` for durable history; assistant text is written incrementally by the proxy until `finish`. While a turn is live, clients should poll with `?limit=N` (tail only) and merge into cached history rather than re-downloading the full thread each tick.

Message `id` values are UUIDs. Roles: `user` | `assistant`.

## Messaging proxy (transport + persistence)

Base: `/api/v1/deployments/:deploymentId/messaging` → sidecar `/api/...`

- `POST /conversations` — only if the client did not pre-assign a conversation id.
- `POST /conversations/:id/messages` — body `{ "content": "..." }`; proxy appends user message to Postgres **before** forwarding upstream. Returns **409** when the message cannot be persisted (assistant reply still streaming, message limit reached, or conversation id conflict) — the message is **not** forwarded upstream in that case.
- SSE: `/conversations/:id/stream` (session cookie; same origin as API in browsers). The proxy mirrors assistant chunks into Postgres (throttled mid-stream, flush on `finish`) even if the browser disconnects. When a client opens SSE, the proxy also consumes upstream in a **detached** context so generation and persistence continue after navigation away or refresh — returning clients catch up via tail polling, not by requiring a new stream consumer. Active sends should still attach SSE so the proxy begins consuming promptly.
- Turn state: the proxy marks the conversation as streaming from the first assistant chunk until `finish`/`error`; clients read it back as `assistant_streaming` on the chat GET and must treat it (or a trailing user message) as "turn in flight" rather than guessing from content growth.

Server injects `X-Amzn-Oidc-Identity` for upstream messaging auth.

## Eligibility (clients)

Use deployment list `messaging_web_configured` (batch DB: messaging sidecar + `http` service) to show chat-capable agents. Before send, gate on `GET …/status` → `active` and `GET …/runtime` → `messaging_reachable` when exposed.

## Storage

Tables: `deployment_chat_conversations`, `deployment_chat_messages` in `sql/astro-server/schema.sql`. Apply via Atlas like other schema changes.

Conversations store a denormalized `agent_name` snapshot and use `ON DELETE SET NULL` for `deployment_id` so history survives deployment removal and can be listed or exported at account scope later. The REST API remains deployment-scoped for now; account-level listing is a future surface.

## Consumers

- **astro-client** (`/chat`) — first UI; TanStack Query wrappers in `apps/astro-client/src/api/queries/chat.ts`.
- **Other clients** — call the same REST paths; do not duplicate history in local-only storage except optional offline cache synced from the server.

OpenAPI: `/openapi.json`, tag **Chat**.
