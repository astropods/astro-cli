# Deployment chat client messaging

**Hot path (v1):** in-session history via `use-deployment-chat.ts` — accumulate SSE chunks locally. Server chat API is stubbed pending Langfuse-backed history. See `docs/04-guides/deployment-chat.md`.

| Module | Runtime |
|--------|---------|
| `transport.ts` | SSE + poll stability helpers |
| `chat-message-adapter.ts` | `ChatMessage[]` → assistant-ui thread messages |
