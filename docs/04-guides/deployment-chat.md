# Deployment chat (platform API)

Canonical transport for **any** Astro client where a signed-in user messages a deployed agent (web, CLI, mobile, etc.). The deployment **messaging sidecar** handles send + SSE stream.

**Durable history is disabled.** Postgres persistence was removed pending Langfuse-backed history (user/agent content should not live in astro-server RDS). Clients keep in-session thread state locally; the chat API routes remain as no-op stubs for forward compatibility.

## Responsibilities

| Layer | Owns |
|-------|------|
| `GET/PUT/POST /api/v1/deployments/:id/chat/...` | Stub API (empty history) — TODO: Langfuse-backed history |
| `POST/GET /api/v1/deployments/:id/messaging/...` | Proxy to sidecar: create conversation (optional), send message, SSE stream |

Clients **read** in-session history from local state (SSE chunks). **TODO:** replace with Langfuse trace reads keyed by `conversation_id` metadata.

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

1. Optional `PUT` conversation (title) — no-op until Langfuse history ships.
2. On user send: messaging `POST` message → messaging SSE for live chunks.
3. Accumulate assistant text from SSE in client state for the current session.

Message `id` values are UUIDs. Roles: `user` | `assistant`.

## Messaging proxy (transport + persistence)

Base: `/api/v1/deployments/:deploymentId/messaging` → sidecar `/api/...`

- `POST /conversations` — only if the client did not pre-assign a conversation id.
- `POST /conversations/:id/messages` — body `{ "content": "..." }`; proxy forwards upstream (no server-side history write).
- SSE: `/conversations/:id/stream` (session cookie; same origin as API in browsers). Clients attach during an active send to receive assistant chunks.
- Turn state: clients track in-flight turns from local SSE state until Langfuse-backed `assistant_streaming` returns on the chat GET.

Server injects `X-Amzn-Oidc-Identity` for upstream messaging auth.

## Eligibility (clients)

Use deployment list `messaging_web_configured` (batch DB: messaging sidecar + `http` service) to show chat-capable agents. Before send, gate on `GET …/status` → `active` and `GET …/runtime` → `messaging_reachable` when exposed.

## Storage

**TODO:** Langfuse-backed history — tag traces with `conversation_id`, read input/output from the account's Langfuse project. Do not store free-form user content in astro-server Postgres.

The `/chat` REST routes remain registered but return empty/no-op responses until that lands.

## Consumers

- **astro-client** (`/chat`) — first UI; TanStack Query wrappers in `apps/astro-client/src/api/queries/chat.ts`.
- **Other clients** — call the same REST paths; do not duplicate history in local-only storage except optional offline cache synced from the server.

OpenAPI: `/openapi.json`, tag **Chat**.
