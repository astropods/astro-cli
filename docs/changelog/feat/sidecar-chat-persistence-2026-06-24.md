# Move chat persistence to the messaging sidecar (no chat data in RDS)

## Summary

Deployment chat is no longer persisted in astro-server or its RDS database.
Previously, conversation metadata (sidebar list, titles, recency, soft-delete)
lived in Postgres and the messaging proxy mirrored user sends and assistant
streams into a Postgres message table. That meant chat content and per-user
conversation metadata sat in the central platform database.

Chat persistence now lives entirely inside each deployment's messaging sidecar,
backed by a SQLite database (WAL mode) on the agent's shared persistent disk
(the default-shared-disk work mounts that disk into the sidecar at `/data` under
the `messaging` subPath). The sidecar has no Langfuse access; durability comes
from that disk, which survives pod reschedules. This PR only points the sidecar
at it via `CHAT_DB_PATH`; it does not provision storage itself.

This satisfies a GDPR data-minimization goal: no conversation metadata or
message bodies are stored in RDS, and astro-server only ever sees chat content
in transit while proxying authenticated requests.

## Design

The chat data path now has a clear split of responsibilities:

- astro-server is an authenticated, in-transit proxy. The existing client API
  (`/deployments/:id/chat/*`) is unchanged in shape; each handler verifies the
  WorkOS session, injects the user id as the `X-Amzn-Oidc-Identity` header, and
  forwards the request to the deployment's messaging sidecar. The messaging
  proxy (`/deployments/:id/messaging/*`) is now a pure passthrough — it no
  longer reads request bodies or tees SSE streams into any database.

- The messaging sidecar owns persistence. It serves the chat-page contract from
  a new SQLite store (`internal/store/sqlite`, WAL mode): conversations (id, user
  id, title, recency, soft-delete) and messages (role, content, contiguous seq).
  The web adapter persists the user turn on send and the assistant turn on the
  terminal stream chunk. New sidecar routes back the chat page:
  `GET /api/chat/conversations`, `GET`/`DELETE /api/chat/conversations/{id}`, and
  `PUT /api/chat/conversations/{id}/title`. The title route is a scoped, idempotent
  PUT on the `/title` sub-resource — renaming an existing, caller-owned
  conversation (it cannot create a conversation or mutate messages), rather than a
  broad `PUT` on the whole conversation resource. The sidecar makes no calls to
  Langfuse.

- Durability rides on the shared agent disk. This PR does not provision a chat
  PVC; it sets `CHAT_DB_PATH=/data/chat.db` on the sidecar so SQLite writes onto
  the agent's shared persistent disk, which the default-shared-disk change (now in
  `main`) mounts into the sidecar at `/data` under the `messaging` subPath.
  History survives pod reschedules.

- Titles and soft-deletes are durable. Because the store lives on the persistent
  volume, renamed titles and soft-deletes persist across reschedules — no
  resurrection.

- Delete does not touch Langfuse. The agent-run traces in Langfuse are telemetry
  powering cost/usage and observability analytics, which a user must not be able
  to wipe by deleting a chat thread (and the sidecar has no Langfuse access in any
  case). Delete soft-deletes only the chat record on the volume.

`assistant_streaming` is no longer a server-authoritative flag; the sidecar
derives it from whether the latest persisted message is still the user's, which
matches the client's existing in-flight detection.

The `deployment_chat_conversations` and `deployment_chat_messages` tables are
removed from the schema, and the astro-server `chatstore` package is deleted.

## Trade-offs

- Single-writer. SQLite (WAL) on the shared ReadWriteOnce agent disk is
  single-writer by design, so the chat store assumes one messaging writer. This
  is fine because agents are single-replica by default (`agent.distributed` is
  opt-in and only gates `replicas > 1`); concurrent multi-pod chat is out of
  scope and deferred until distributed agents are actually built out.
- If the live SSE connection drops and the client falls back to polling, partial
  assistant text is not visible until the turn completes (the assistant message
  is persisted on stream completion).

## Migration

Apply the updated schema before deploying so the dropped chat tables are removed.
No infrastructure or Terraform changes are required — the agent's shared disk
(default-shared-disk, already in `main`) provides the durable mount at `/data`.
No action is required from end users.
