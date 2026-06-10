# Deployment chat client messaging

**Hot path (v1):** server-owned history via `use-deployment-chat.ts` — poll `GET …/chat/conversations/:id`, open SSE only during an active send. See `docs/04-guides/deployment-chat.md`.

| Module | Runtime |
|--------|---------|
| `transport.ts` | SSE + poll stability helpers |
| `chat-message-adapter.ts` | `ChatMessage[]` → assistant-ui thread messages |
