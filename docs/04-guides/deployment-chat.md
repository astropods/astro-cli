# Deployment chat (platform API)

Canonical transport for **any** Astro client where a signed-in user messages a deployed agent (web, CLI, mobile, etc.). The deployment **messaging sidecar** handles send + SSE stream **and owns all chat persistence**.

**No chat data is stored in astro-server or RDS.** Conversation metadata (list, title, recency, soft-delete) and message bodies live in the messaging sidecar's SQLite database (WAL mode) on the agent's shared persistent disk, mounted into the sidecar at `/data` by the default-shared-disk wiring. The disk survives pod reschedules. The sidecar has no Langfuse access. astro-server only ever sees chat content in transit while proxying authenticated requests.

## Responsibilities

| Layer | Owns |
|-------|------|
| `GET/POST/DELETE /api/v1/deployments/:id/chat/...` | astro-server: authenticate session, inject user id, forward to sidecar `/api/chat/...` (in transit only — no persistence) |
| `POST/GET /api/v1/deployments/:id/messaging/...` | astro-server: pure proxy to sidecar (create conversation, send message, SSE stream) |
| messaging sidecar | Chat persistence (SQLite on a shared persistent volume), chat-page contract endpoints |

## Auth and scope

- Same session as the rest of the Astro API (WorkOS user in middleware).
- Deployment ownership: caller must be a member of the deployment's account (`resolveDeployment`).
- astro-server injects `X-Amzn-Oidc-Identity` (the WorkOS user id) on every forwarded request; the sidecar uses it as the ownership key.
- History is keyed by **`user_id`** within a deployment's sidecar. Two users on the same account do not share threads.

## Conversation identity

- Clients choose a **UUID v4** `conversation_id` before the first message (recommended), or the sidecar assigns one on `POST /messaging/conversations`.
- Use that **same id** for:
  - `POST /messaging/conversations/:conversationId/messages` (send)
  - `GET /messaging/conversations/:conversationId/stream` (SSE)
  - `POST /messaging/conversations/:conversationId/cancel` (stop generating)
  - `PUT /chat/conversations/:conversationId/title` (rename)
- `conversation_id` is emitted by agents as the Langfuse `session.id` (for observability), and the WorkOS user id as the Langfuse `user.id`. The chat store keys conversations and messages by `conversation_id` + user id independently of Langfuse.

## Chat API (history)

Base: `/api/v1/deployments/:deploymentId/chat`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/conversations` | List the user's conversations (most recent first). `assistant_streaming` is true when the latest message is still the user's. |
| GET | `/conversations/:conversationId` | Thread (`messages[]`) plus `assistant_streaming`. Omit `limit` for full history; `?limit=N` returns the tail, `?before_seq=S` an older page; response includes `has_more` and `oldest_seq` when paginated. |
| PUT | `/conversations/:conversationId/title` | Rename an existing conversation (`{ "title": "..." }`; non-empty, max 80 chars). Idempotent and title-only: it cannot create a conversation or change messages. Returns **404** when the conversation is missing or not owned by the caller. |
| DELETE | `/conversations/:conversationId` | Soft-delete: hides the conversation from the owning user's list. Durable (persists across reschedules via the volume). Does **not** touch Langfuse — those traces are cost/usage telemetry, not the chat store. |

These are served by the sidecar from SQLite on the shared persistent volume, so they survive pod reschedules without any Langfuse access.

Message `id` values are UUIDs. Roles: `user` | `assistant`.

**Typical client flow**

1. On user send: `POST /messaging/conversations` (if no id yet) → `POST /messaging/conversations/:id/messages` → attach `GET /messaging/conversations/:id/stream` for live chunks.
2. To **stop generating** mid-reply: `POST /messaging/conversations/:id/cancel`. The turn ends immediately and the partial the user saw is persisted.
3. `PUT /chat/conversations/:id/title` to rename it (the conversation already exists from the send in step 1).
4. Read durable history with `GET /chat/conversations/:id`.

## Messaging proxy (transport only)

Base: `/api/v1/deployments/:deploymentId/messaging` → sidecar `/api/...`

- `POST /conversations` — create; sidecar persists the conversation row.
- `POST /conversations/:id/messages` — body `{ "content": "..." }`; sidecar persists the user message and forwards to the agent. Returns **404** for a conversation owned by another user, and **409** (`message_limit_reached`) once the thread hits its per-conversation message cap.
- SSE `/conversations/:id/stream` — assistant chunks; the sidecar persists the assistant turn progressively during the stream and on completion.
- `POST /conversations/:id/cancel` — **stop generating**: ends the in-flight turn, persists the partial the user saw, sends a terminal `finish` on the SSE stream, and best-effort signals the agent to abort (agents that ignore it keep running, but their output is dropped). Idempotent — a no-op when no turn is active. **404** for a missing/foreign conversation.

This proxy never reads or stores chat content on astro-server — it is a byte passthrough with the identity header injected.

## Eligibility (clients)

Use deployment list `messaging_web_configured` (batch DB: messaging sidecar + `http` service) to show chat-capable agents. Before send, gate on `GET …/status` → `active` and `GET …/runtime` → `messaging_reachable` when exposed.

## Storage

- **Sidecar SQLite** (`CHAT_DB_PATH=/data/chat.db`, WAL mode): the store for conversations and messages, including titles, recency, and soft-deletes. When `CHAT_DB_PATH` is unset (e.g. local dev), persistence is disabled and the chat endpoints return empty/no-op responses (list is empty; get/rename/delete of a specific conversation is a 404/204).
- **Shared agent disk**: `/data` is the agent's persistent disk, mounted into the sidecar (under the `messaging` subPath) by the default-shared-disk wiring — not provisioned by the chat code. It survives pod reschedules. This feature only sets `CHAT_DB_PATH`; durable history depends on that shared disk being mounted.
- **RDS**: stores no chat metadata or message content.
- **Langfuse**: not used by the sidecar at all. Agent traces still flow to Langfuse via the collector for observability, but the chat store does not read or write Langfuse.

Caveat: SQLite (WAL) on the ReadWriteOnce agent disk is single-writer, so the chat store assumes a single messaging writer. Agents are single-replica by default (`agent.distributed` is opt-in and only unlocks `replicas > 1`), so this is fine for typical use; concurrent multi-pod chat is out of scope.

Delete is a soft delete and is durable (persists across reschedules via the disk). It deliberately does **not** touch Langfuse — those traces are agent-run telemetry behind cost/usage and observability analytics, which a user must not be able to wipe by removing a chat thread.

## Consumers

- **astro-client** (`/chat`) — first UI; TanStack Query wrappers in `apps/astro-client/src/api/queries/chat.ts`.
- **Other clients** — call the same REST paths.

OpenAPI: `/openapi.json`, tag **Chat**.
