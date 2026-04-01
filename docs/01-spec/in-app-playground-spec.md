# In-App Playground Spec

The current `ast playground <url>` CLI experience opens the playground UI in a Docker container on localhost, requiring a port-forward for local deployments and breaking the user out of the Astro UI entirely. This spec covers an in-app playground panel accessible from a persistent button on the agent detail page, working uniformly across local, preview, and prod.

## Architecture

The playground does not talk to the agent directly. The deployed agent stack includes a **messaging service** container that acts as the gateway between browser clients (HTTP/SSE) and the agent (gRPC). The messaging service exposes a JSON/SSE HTTP API on port 8080:

```
POST /api/conversations                      → create conversation, returns { conversation_id }
POST /api/conversations/{id}/messages        → send user message, returns { message_id }
GET  /api/conversations/{id}/stream          → SSE stream of agent responses
```

In every environment (local K8s, preview, prod) the messaging service runs as a ClusterIP service — reachable from the astro-server but not from the browser. The server therefore acts as a thin proxy for all playground traffic.

## 1. Server: Playground Proxy

New handler file `apps/astro-server/handlers/playground.go` registers three routes:

```
POST /api/v1/deployments/:id/playground/conversations
POST /api/v1/deployments/:id/playground/conversations/:convId/messages
GET  /api/v1/deployments/:id/playground/conversations/:convId/stream
```

Each handler:
1. Auth + membership check (same middleware chain as existing deployment handlers)
2. Looks up the deployment by `:id`, finds the service endpoint with `name == "messaging"` in `service_endpoints`
3. Proxies the request body to `http://{messaging-url}/api/...` using the deployment's internal ClusterIP URL
4. For the stream endpoint: sets `Content-Type: text/event-stream`, `X-Accel-Buffering: no`, flushes each SSE event as it arrives from upstream

The messaging URL comes from the `ServiceEndpoint` already stored in the deployment record (`internal/deployment/types.go`). No K8s calls needed at proxy time.

Register routes in the existing router alongside other deployment routes.

## 2. Client: API Layer

New file `apps/astro-client/src/api/queries/playground.ts`:

```ts
useCreateConversation(deploymentId: string)   // mutation → POST /playground/conversations
useSendMessage(deploymentId: string)          // mutation → POST /playground/conversations/:id/messages
```

SSE streaming is handled outside TanStack Query using `fetch` with `ReadableStream` (not `EventSource`) — required to pass Astro auth headers through the server proxy. A `usePlaygroundStream(deploymentId, convId)` hook manages the stream lifecycle: open on send, close on completion or error, expose accumulated `text` and `isStreaming` state.

Add `playground` factory to `apps/astro-client/src/api/queries/keys.ts`.

Add API function types (`CreateConversationResponse`, `SendMessageResponse`) to `apps/astro-client/src/lib/api.ts`.

## 3. Client: PlaygroundPanel

New component `apps/astro-client/src/components/deployed-agent/detail/playground/PlaygroundPanel.tsx`.

Uses the same panel shell as `ConfigurePanel` and `TraceDetailPanel`:

```
PANEL_SHELL_CLASS  = "flex h-full w-[420px] flex-col border-l border-border bg-surface"
PANEL_HEADER_CLASS = "flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5"
```

Layout:
- Header: "Playground" label + close button
- Body: scrollable message list (user bubbles right-aligned, agent bubbles left-aligned)
- Footer (sticky): text input + Send button

Behavior:
- On panel open: call `useCreateConversation` to create a new session; store `convId` in component state
- On send: call `useSendMessage`, then open SSE stream via `usePlaygroundStream`; append a streaming assistant bubble that fills as chunks arrive
- Input and Send disabled while `isStreaming`; re-enabled on stream close or error
- Conversation state resets when the panel is closed and reopened

## 4. Client: Playground Button + State Wiring

In `apps/astro-client/src/components/deployed-agent/detail/ActiveDetailView.tsx`:

Add `playgroundOpen` state alongside existing `configOpen`. Update `panelOpen` derived value to include `playgroundOpen`. Right panel slot renders `<PlaygroundPanel>` when `playgroundOpen`, `<ConfigurePanel>` when `configOpen` — mutually exclusive, opening one closes the other.

Add a **Playground** button to the header action group (between Restart and Configure). The button uses `ChatBubbleLeftRightIcon` (Heroicons). It is always rendered; disabled with tooltip `"No messaging endpoint available"` when the deployment has no `service_endpoint` with `name == "messaging"`.

## Key Decisions

- **Server proxy for all envs**: avoids CORS, browser-to-ClusterIP routing issues, and any env-specific branching in the client
- **No OpenAI API**: the stack uses its own messaging service HTTP/SSE protocol
- **Conversation-per-session**: new `conversation_id` on each panel open; no cross-session history (follow-up)
- **fetch streaming over EventSource**: EventSource cannot send custom headers; auth headers are needed for the proxy

## Verification

1. Deploy an agent with a messaging container locally
2. Open agent detail page → Playground button visible in header
3. Deployment with no messaging endpoint → button disabled with tooltip
4. Click Playground → panel slides in; ConfigurePanel closes if open
5. Send a message → response streams token-by-token into the chat
6. Close and reopen panel → fresh conversation starts
7. Mobile/compact (< 1180px) → panel follows existing responsive behavior
