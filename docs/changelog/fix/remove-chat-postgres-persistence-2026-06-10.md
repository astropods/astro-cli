# Remove deployment chat Postgres persistence

## Summary

Deployment chat was writing user prompts and assistant replies into astro-server Postgres (`deployment_chat_messages`). That duplicates content already destined for Langfuse, lacks the tenancy isolation we want for user data, and adds heavy write load to the platform RDS. This change removes that persistence until history is backed by Langfuse.

## Design

**Server:** Delete `chatstore` and the `deployment_chat_*` schema tables. Chat handlers remain registered but return empty lists / no-op writes so API contracts stay stable. The messaging proxy is transport-only again (no send-body tee, no detached SSE mirroring).

**Client:** `use-deployment-chat` keeps in-session thread state locally and renders assistant text from SSE chunks. Session sidebar uses ephemeral in-memory state for the current browser session.

**TODO:** Langfuse-backed history — read traces tagged with `conversation_id` from the account Langfuse project; optional thin metadata layer for titles only.

## Migration

No action required. Environments that already applied the chat tables can drop `deployment_chat_messages` and `deployment_chat_conversations` manually or via the updated `schema.sql`. Chat works in-session; refresh clears thread history until Langfuse persistence ships.
