# astro-messaging Architecture

## Overview

`astro-messaging` is a Go sidecar service that sits between messaging platforms (Slack, web browsers) and AI agent containers. It translates platform-specific events into a unified protobuf format and forwards them to agents over gRPC bidirectional streams. It is a thin protocol translation layer with no business logic — agents own all conversation context, AI processing, and response generation.

```
User (Slack / Web Browser)
    |
Platform Adapter (Socket Mode / HTTP+SSE)
    |
Messaging Sidecar (gRPC Server)
    |  Bidirectional gRPC Stream
Agent Container (gRPC Client)
    |
AI/LLM Processing
```

## Directory Structure

```
packages/astro-messaging/
├── cmd/sidecar/main.go                  # Entry point, wires all components
├── config/config.go                     # Env-var based configuration
├── proto/astro/messaging/v1/            # Protobuf source definitions
│   ├── message.proto                    # Message, User, PlatformContext, Attachment
│   ├── response.proto                   # AgentResponse, ContentChunk, StatusUpdate, ThreadHistory
│   ├── feedback.proto                   # PlatformFeedback (edits, deletes, reactions)
│   ├── config.proto                     # AgentConfig, tool graph
│   └── service.proto                    # AgentMessaging gRPC service
├── pkg/
│   ├── gen/astro/messaging/v1/          # Generated Go protobuf + gRPC code
│   ├── client/go/                       # Go SDK for agents
│   │   └── messaging_client.go
│   └── types/message.go                 # Shared Go types (ConversationContext, etc.)
├── src/messaging-client.ts              # TypeScript SDK for agents
├── internal/
│   ├── adapter/
│   │   ├── adapter.go                   # Adapter interface + Config types
│   │   ├── capabilities.go              # AdapterCapabilities + platform presets
│   │   ├── slack/
│   │   │   ├── adapter.go               # SlackAdapter: lifecycle, event dispatch
│   │   │   ├── ai_features.go           # HandleAgentResponse routing
│   │   │   ├── slack_ai_api.go          # HTTP client for Slack AI APIs
│   │   │   └── rate_limiter.go          # Token bucket rate limiter
│   │   └── web/
│   │       ├── adapter.go               # WebAdapter: HTTP server + SSE
│   │       ├── handlers.go              # REST endpoint handlers
│   │       ├── connection.go            # SSE ConnectionManager
│   │       ├── events.go                # SSE event types + factories
│   │       └── session.go               # SessionManager interface + implementations
│   ├── grpc/
│   │   └── server.go                    # gRPC server (AgentMessaging service)
│   ├── store/
│   │   ├── store.go                     # ConversationStore interface
│   │   ├── memory.go                    # In-memory ConversationStore
│   │   ├── redis.go                     # Redis ConversationStore
│   │   ├── thread_history.go            # ThreadHistoryStore (platform thread cache)
│   │   └── agent_config.go             # AgentConfigStore (in-memory singleton)
│   └── version/version.go              # Build version via ldflags
└── docs/
    └── architecture.md                  # This file
```

## Adapter Interface

Every platform adapter implements this single interface:

```go
type Adapter interface {
    Initialize(ctx context.Context, config Config) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    GetPlatformName() string
    IsHealthy(ctx context.Context) bool
    Capabilities() AdapterCapabilities
    SetMessageHandler(handler MessageHandler)
    HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error
    HydrateThread(ctx context.Context, conversationID string, store *store.ThreadHistoryStore) error
}

type MessageHandler func(ctx context.Context, msg *pb.Message) error
```

All messages use protobuf types (`pb.Message`, `pb.AgentResponse`) — there is no secondary message format. The `MessageHandler` is the connection point between adapters and the gRPC server: in production, `grpcServer.HandleIncomingMessage` is registered as the handler.

### AdapterCapabilities

Each adapter declares what the platform supports:

```go
type AdapterCapabilities struct {
    SupportsStreaming        bool     // Token-by-token content streaming
    SupportsStatusUpdates    bool     // "Thinking...", "Searching..." indicators
    SupportsSuggestedPrompts bool     // Quick reply suggestions
    SupportsThreads          bool     // Native threading
    SupportsTypingIndicator  bool     // Typing indicator
    MaxUpdateRateHz          float64  // Max update frequency (0 = unlimited)
    MaxContentLength         int      // Max message length (0 = unlimited)
    SupportsReactions        bool     // Emoji reactions
    SupportsCards            bool     // Rich card attachments
}
```

**Slack**: streaming=false, status=true, prompts=true, threads=true, rate=0.33Hz (3s), maxLen=4000
**Web**: streaming=true, status=true, prompts=true, threads=true, rate=unlimited, maxLen=unlimited

## Protobuf Wire Format

Five proto files under `proto/astro/messaging/v1/` define the entire wire format.

### message.proto — Inbound Messages

`Message` is the canonical user message flowing from platform to agent:
- `id` — UUID generated by the sidecar
- `timestamp`, `platform` ("slack", "web")
- `platform_context` — platform-specific IDs (message_id, channel_id, thread_id, workspace_id, platform_data map)
- `user` — id, username, avatar_url, email, user_data map
- `content` — cleaned text (mentions stripped)
- `attachments` — typed (IMAGE, FILE, VIDEO, AUDIO, LINK) with url, filename, size, mime_type
- `conversation_id` — stable routing key for the full conversation lifecycle

### response.proto — Agent Responses + Thread History

`AgentResponse` wraps everything the agent sends back:
- `conversation_id`, `response_id`
- oneof `payload`:
  - `incoming_message` (Message) — server pushes platform messages to the agent
  - `status` (StatusUpdate) — THINKING, SEARCHING, GENERATING, PROCESSING, ANALYZING, CUSTOM
  - `content` (ContentChunk) — START/DELTA/END/REPLACE streaming pattern
  - `prompts` (SuggestedPrompts) — clickable prompt suggestions
  - `thread_metadata` (ThreadMetadata) — thread title, creation
  - `error` (ErrorResponse) — RATE_LIMIT, CONTEXT_TOO_LONG, AGENT_ERROR, TOOL_ERROR, etc.
  - `context_request` (ThreadHistoryRequest) — agent requests thread history

`ThreadHistoryResponse` / `ThreadMessage` — for hydrating platform thread state including edits and deletions.

### feedback.proto — Platform Feedback

`PlatformFeedback` carries user actions back to the agent:
- `StreamControl` — STOP, PAUSE, RESUME, REGENERATE
- `PromptSelection` — user clicked a suggested prompt
- `MessageReaction` — thumbs up/down, custom emoji
- `ButtonClick` — interactive button
- `MessageEdit` / `MessageDelete` — user edited or deleted a message

### config.proto — Agent Configuration

`AgentConfig` carries agent metadata (system prompt, tool definitions with optional graph visualization) from agent to sidecar.

### service.proto — gRPC Service

```protobuf
service AgentMessaging {
  rpc ProcessConversation(stream ConversationRequest) returns (stream AgentResponse);
  rpc ProcessMessage(Message) returns (stream AgentResponse);
  rpc GetThreadHistory(ThreadHistoryRequest) returns (ThreadHistoryResponse);
  rpc GetConversationMetadata(ConversationMetadataRequest) returns (ConversationMetadataResponse);
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

`ConversationRequest` (agent → sidecar on the bidi stream) is a oneof of: `message`, `feedback`, `agent_config`, `agent_response`.

## gRPC Server

The central hub (`internal/grpc/server.go`) bridges adapters and agents:

```go
type Server struct {
    pb.UnimplementedAgentMessagingServer
    adapters          map[string]adapter.Adapter    // "slack" → SlackAdapter, etc.
    mu                sync.RWMutex                  // protects adapters map
    threadStore       *store.ThreadHistoryStore
    agentConfigStore  *store.AgentConfigStore
    conversationCache store.ConversationStore        // Redis or in-memory
    streams           map[string]*conversationStream // active agent streams
    streamsMu         sync.RWMutex                  // protects streams map
    grpcServer        *grpc.Server
    listenAddr        string
}
```

Server options are set in `Start()`:
- `MaxConcurrentStreams(100)` — max simultaneous bidi streams
- `MaxRecvMsgSize(4MB)` / `MaxSendMsgSize(4MB)` — message size limits
- No TLS — assumes sidecar-to-agent communication is pod-local

### ProcessConversation — The Core RPC

Bidirectional streaming between agent and sidecar:

1. Agent connects, sends first message to register the stream
2. Server registers stream in `streams` map (keyed by stream ID)
3. Receive loop handles agent messages:
   - `ConversationRequest_Message` — wraps as `AgentResponse_Content{END}` and routes to platform
   - `ConversationRequest_AgentResponse` — routes typed response to platform via `routeAgentResponse()`
   - `ConversationRequest_AgentConfig` — stores in AgentConfigStore
   - `ConversationRequest_Feedback` — logged only (no routing)

### HandleIncomingMessage — Platform → Agent

Called by adapters when a message arrives:

1. Updates `conversationCache` (creates or increments message count)
2. Iterates `streams` map and picks the **first** stream encountered (Go map iteration is non-deterministic — single-agent limitation; multi-agent routing not yet implemented)
3. Wraps message as `AgentResponse{IncomingMessage: msg}` and sends to agent

### routeAgentResponse — Agent → Platform

1. Looks up `conversationID` in `conversationCache` to find the platform
2. If found, routes to the specific adapter's `HandleAgentResponse()`
3. If not found, broadcasts to all registered adapters

### Other RPCs

- **GetThreadHistory** — checks staleness (`getRefreshInterval()` returns `5 * time.Minute`), hydrates from platform if stale, returns from ThreadHistoryStore
- **GetConversationMetadata** — lookups by conversation_id or platform_id in conversation cache
- **ProcessMessage** — stub (logs and returns nil); unary RPC not used in production
- **HealthCheck** — calls `IsHealthy()` on all adapters; returns HEALTHY or DEGRADED with hardcoded version `"1.0.0"`

## Slack Adapter

`internal/adapter/slack/` implements the Slack platform adapter.

### Event Flow

```
Slack WebSocket (Socket Mode)
  → socketmode.Event
    → handleSocketEvent()
      → EventTypeEventsAPI → Ack + handleInnerEvent()
        → MessageEvent → handleMessage()
        → AppMentionEvent → handleAppMention()
      → EventTypeInteractive → Ack + handleBlockActions()
```

### Message Handling

- **DM messages** (`handleMessage`): only processes channels starting with 'D' (DMs). Channel messages are handled by `app_mention` to avoid duplicates. Builds `pb.Message` directly and calls `msgHandler`.
- **App mentions** (`handleAppMention`): strips `<@USERID>` mentions from text, uses `ev.ThreadTimeStamp` or falls back to `ev.TimeStamp` as thread ID. Sets "thinking" status immediately via `aiClient.SetThreadStatus()`.

### Conversation ID Format

- DMs: `channelID` (e.g., `D1234567`)
- Threaded channel messages: `channelID-threadTS` (e.g., `C1234567-1234567890.000001`)

### Agent Response Handling (`ai_features.go`)

`HandleAgentResponse` routes `pb.AgentResponse` payloads:

| Payload            | Handler                  | Slack API                                                     |
| ------------------ | ------------------------ | ------------------------------------------------------------- |
| `StatusUpdate`     | `setSlackStatus()`       | `assistant.threads.setStatus` (via `SlackAIClient`)           |
| `ContentChunk`     | `handleContentChunk()`   | `chat.postMessage` (on END only)                              |
| `SuggestedPrompts` | `setSlackPrompts()`      | `assistant.threads.setSuggestedPrompts` (via `SlackAIClient`) |
| `ThreadMetadata`   | `handleThreadMetadata()` | Logged only                                                   |
| `ErrorResponse`    | `handleError()`          | `chat.postMessage` with warning emoji                         |

### Content Buffering Strategy

Slack's 3-second API rate limit makes token-by-token streaming a poor UX. Instead:

- `START` — resets `contentBuffers[conversationID]`
- `DELTA` — appends to buffer
- `END` — flushes the complete message as a single `PostMessageContext` call, then deletes the buffer
- `REPLACE` — sends content immediately as a new message

Status updates ("Thinking...", "Searching...") provide user-visible progress while the buffer accumulates.

### SlackAIClient (`slack_ai_api.go`)

Custom HTTP client for Slack AI APIs not in the `slack-go` library. Struct holds `botToken`, `httpClient`, and `baseURL` (defaults to `https://slack.com/api`).

Methods:
- `SetThreadStatus(ctx, channelID, threadTS, status, emoji)` → `assistant.threads.setStatus`
- `SetSuggestedPrompts(ctx, channelID, threadTS, prompts)` → `assistant.threads.setSuggestedPrompts`
- `SetTitle(ctx, channelID, threadTS, title)` → `assistant.threads.setTitle`
- `PostMessageWithFeedback(ctx, channelID, content, threadID)` → `chat.postMessage` with Block Kit

The `SuggestedPrompt` struct has `Title` and `Message` fields only (no ID/Description unlike the proto definition).

`PostMessageWithFeedback` sends two Block Kit blocks:
1. `section` block with `mrkdwn` text containing the message content
2. `context_actions` block with `feedback_buttons` action containing 👍 (`positive_feedback`) and 👎 (`negative_feedback`) buttons

### Rate Limiter

Token bucket implementation (`rate_limiter.go`): configurable `requestsPerSecond` and `burstSize`. Bucket starts full (`tokens = burstSize`). `Wait(ctx)` polls every 100ms calling `refill()` which computes elapsed time since last refill, adds `elapsed * refillRate` tokens capped at `maxTokens`, and consumes one token if available. Blocks until a token is available or context is cancelled. Default for Slack: 3.0 RPS, burst 10.

### Thread Hydration

`HydrateThread()` parses the conversation ID (`slack-channelID-threadTS`), calls `GetConversationReplies` or `GetConversationHistory`, converts Slack messages to `pb.ThreadMessage` protos, and stores them in the passed `ThreadHistoryStore`.

### Feedback Buttons

When a user clicks a feedback button (thumbs up/down), `handleBlockActions()` removes the feedback button blocks from the message and adds a reaction emoji via the Slack API.

## Web Adapter

`internal/adapter/web/` serves browser clients via HTTP REST + Server-Sent Events.

### HTTP Routes

| Method | Path                               | Handler                  | Purpose                                              |
| ------ | ---------------------------------- | ------------------------ | ---------------------------------------------------- |
| POST   | `/api/conversations`               | HandleCreateConversation | Create new conversation (returns UUID)               |
| POST   | `/api/conversations/{id}/messages` | HandleSendMessage        | Send message (builds `pb.Message`, forwards to gRPC) |
| GET    | `/api/conversations/{id}/stream`   | HandleStream             | SSE connection for real-time events                  |
| GET    | `/api/conversations/{id}/history`  | HandleHistory            | Fetch thread history from ThreadHistoryStore         |
| GET    | `/api/agent/config`                | HandleAgentConfig        | Get agent configuration (tools, system prompt)       |
| GET    | `/health`                          | HandleHealth             | Health check                                         |

### SSE Events

Event type constants defined in `events.go`:

| Constant              | Value             | Status                             |
| --------------------- | ----------------- | ---------------------------------- |
| `EventConnected`      | `connected`       | Active — sent on stream open       |
| `EventChunk`          | `chunk`           | Active — content streaming         |
| `EventStatus`         | `status`          | Active — status indicators         |
| `EventFinish`         | `finish`          | Active — marks response complete   |
| `EventError`          | `error`           | Active — error reporting           |
| `EventHeartbeat`      | `heartbeat`       | Active — keepalive                 |
| `EventPrompts`        | `prompts`         | Active — suggested prompts         |
| `EventStepStart`      | `step-start`      | Reserved — no factory function yet |
| `EventStepEnd`        | `step-end`        | Reserved — no factory function yet |
| `EventReasoningStart` | `reasoning-start` | Reserved — no factory function yet |
| `EventReasoningDelta` | `reasoning-delta` | Reserved — no factory function yet |
| `EventReasoningEnd`   | `reasoning-end`   | Reserved — no factory function yet |

The web adapter converts `pb.AgentResponse` payloads to SSE events:

| Proto Payload           | SSE Event Type     | Description                   |
| ----------------------- | ------------------ | ----------------------------- |
| `ContentChunk(START)`   | `chunk`            | Start of content stream       |
| `ContentChunk(DELTA)`   | `chunk`            | Incremental content           |
| `ContentChunk(END)`     | `chunk` + `finish` | Final content + finish signal |
| `ContentChunk(REPLACE)` | `chunk`            | Full content replacement      |
| `StatusUpdate`          | `status`           | Status indicator              |
| `SuggestedPrompts`      | `prompts`          | Clickable prompt suggestions  |
| `ErrorResponse`         | `error`            | Error with code and message   |

Unlike Slack, the web adapter streams every content chunk directly to the client (no buffering). This enables real-time token streaming in the browser.

#### SSE Data Structures

Each event carries a JSON payload with a `type` discriminator field:

- **`ChunkEventData`**: `type`, `content`, `chunk_type` (start/delta/end/replace), `response_id?`, `platform_message_id?`
- **`StatusEventData`**: `type`, `status`, `message?`, `emoji?`
- **`ConnectedEventData`**: `type`, `conversation_id`, `connection_id`
- **`FinishEventData`**: `type`, `response_id?`
- **`ErrorEventData`**: `type`, `code`, `message`, `details?`, `retryable`
- **`PromptsEventData`**: `type`, `prompts[]` where each prompt has `id`, `title`, `message`, `description?`
- **`StepEventData`**: `type`, `step_id`, `name?`, `details?`

### Connection Manager

`ConnectionManager` tracks SSE connections per conversation. Each `SSEConnection` has a buffered channel (`make(chan SSEEvent, 100)`). `Broadcast(conversationID, event)` does a non-blocking send to all connections for that conversation — if a connection's channel is full, the event is **silently dropped** with a log warning. A heartbeat goroutine sends `{"type":"heartbeat"}` events every 30 seconds; heartbeats dropped on full channels are silently ignored (no log).

### HTTP Server

The web adapter's HTTP server uses these timeouts:
- `ReadTimeout: 10s` — max time to read request headers/body
- `WriteTimeout: 0` — disabled for SSE (long-lived connections)
- `IdleTimeout: 120s` — max time between requests on a keep-alive connection

### Session Management

Pluggable via `SessionManager` interface with `ValidateRequest(ctx, *http.Request) (*Session, error)`.

```go
type Session struct {
    UserID    string
    Username  string
    Email     string
    AvatarURL string
    Metadata  map[string]string
}
```

Three implementations:
- **`NoopSessionManager`** — always returns `{UserID: "anonymous"}` (default, for gateway-level auth)
- **`HeaderSessionManager`** — reads configurable HTTP headers (UserIDHeader, UsernameHeader, EmailHeader, AvatarURLHeader). Returns `nil, nil` if the UserID header is missing (no session, no error).
- **`BearerTokenSessionManager`** — extracts token from `Authorization: Bearer <token>` header, calls injected `ValidateToken(ctx, token)` callback. Returns `nil, nil` if the header is missing or doesn't have the `Bearer ` prefix.

### CORS

`corsMiddleware` checks `allowedOrigins` with support for exact match and `*.domain.com` wildcard subdomains.

## Store Layer

### ConversationStore (`store.go`)

Interface for conversation routing metadata:

```go
type ConversationStore interface {
    Get(ctx, conversationID) (*types.ConversationContext, error)
    Create(ctx, *types.ConversationContext) error
    Update(ctx, *types.ConversationContext) error
    Delete(ctx, conversationID) error
    Close() error
}
```

`ConversationContext` stores: ConversationID, Platform, ChannelID, ThreadID, UserID, CreatedAt, LastMessageAt, MessageCount, Metadata.

**MemoryStore** — `map[string]*ConversationContext` with `sync.RWMutex`. Copy-on-read (Get returns a struct copy) and copy-on-write (Create/Update store a copy, not the caller's pointer) to prevent mutation of stored data.

**RedisStore** — keys `conversation:<id>`, JSON-serialized values. Both `Create()` and `Update()` use `SetEx` which resets the TTL to the full configured duration on every write. `Create()` checks `Exists()` first and returns an error if the conversation already exists. Initializes with a `Ping` using a 5-second timeout.

### ThreadHistoryStore (`thread_history.go`)

In-memory store for platform thread messages — ground truth of what's actually in a Slack/web thread including edits and deletions.

Each thread is stored as a `ThreadHistory` struct: `ConversationID`, `Messages []*pb.ThreadMessage`, `LastFetched time.Time`, `Platform string`.

Core methods:
- `AddMessage(conversationID, msg)` — creates thread if missing, evicts oldest thread (by `LastFetched`) if at capacity, trims to maxMessages
- `UpdateMessage(conversationID, messageID, newContent)` — preserves original content, sets `WasEdited = true`
- `DeleteMessage(conversationID, messageID)` — marks `IsDeleted = true` and sets `DeletedAt` (does NOT physically remove from slice)
- `GetHistory(conversationID, maxMessages, includeDeleted)` — returns filtered/trimmed history
- `IsStale(conversationID, duration)` — true if not found or older than threshold

Additional methods:
- `Exists(conversationID)` — checks if a thread exists
- `Clear(conversationID)` — deletes a thread entirely
- `CleanupStale() int` — removes all threads where `LastFetched` exceeds TTL, returns count removed
- `Stats() ThreadHistoryStats` — returns `TotalThreads` (count) and `TotalMessages` (sum across all threads)

Defaults: max 1000 threads, 50 messages per thread, 24-hour TTL.

### AgentConfigStore (`agent_config.go`)

Simple in-memory singleton holding the latest `*pb.AgentConfig` received from the agent. The web adapter's `/api/agent/config` endpoint reads from this; the gRPC server writes to it when an agent sends its config.

## Data Flow

### Inbound: Platform Message → Agent

```
Slack Socket Mode WebSocket event
  → SlackAdapter.handleMessage() / handleAppMention()
    → builds *pb.Message
    → calls msgHandler() [= grpcServer.HandleIncomingMessage]
      → updates conversationCache
      → wraps as AgentResponse{IncomingMessage: msg}
      → stream.Send() to active agent stream
        → Agent receives AgentResponse.IncomingMessage
```

### Outbound: Agent Response → Platform

```
Agent sends ConversationRequest{AgentResponse: response} on stream
  → grpcServer.ProcessConversation() receive loop
    → routeAgentResponse(ctx, response)
      → conversationCache.Get(conversationID) → finds platform
      → adapter.HandleAgentResponse(ctx, response)
        → Slack: buffer DELTAs, post on END, set status/prompts via AI API
        → Web: broadcast SSE events to connected clients
```

### Thread History Hydration

```
Agent calls GetThreadHistory(conversationID)
  → threadStore.IsStale() → true (>5 min)
    → adapter.HydrateThread(ctx, conversationID, threadStore)
      → Slack: GetConversationReplies → convert to ThreadMessage → store
  → threadStore.GetHistory() → return to agent
```

## Startup Wiring (`cmd/sidecar/main.go`)

```
1. config.Load()
2. Initialize ConversationStore (Redis or Memory)
3. Initialize ThreadHistoryStore, AgentConfigStore
4. Initialize gRPC Server (if enabled)
5. Initialize adapters:
   - Slack: slack.New() → Initialize() → register with gRPC server
   - Web: web.New(opts) → Initialize() → SetThreadStore() → SetAgentConfigStore()
6. Register adapters with gRPC server
7. Wire message handlers: adpt.SetMessageHandler(grpcServer.HandleIncomingMessage)
8. Start gRPC server, start adapters (all in goroutines)
9. Wait for SIGINT/SIGTERM
10. Graceful shutdown: stop gRPC, stop adapters, close stores
```

## Configuration

All configuration is loaded from environment variables (no config file parsing at runtime).

| Setting                     | Env Var                       | Default                      |
| --------------------------- | ----------------------------- | ---------------------------- |
| Log level                   | `LOG_LEVEL`                   | `info`                       |
| gRPC enabled                | `GRPC_ENABLED`                | `true`                       |
| gRPC listen address         | `GRPC_LISTEN_ADDR`            | `:9090`                      |
| gRPC max streams            | `GRPC_MAX_STREAMS`            | `100`                        |
| Slack enabled               | `SLACK_ENABLED`               | `false`                      |
| Slack bot token             | `SLACK_BOT_TOKEN`             | — (required if enabled)      |
| Slack app token             | `SLACK_APP_TOKEN`             | — (required for socket mode) |
| Slack socket mode           | `SLACK_SOCKET_MODE`           | `true`                       |
| Slack auto-thread           | `SLACK_AUTO_THREAD`           | `true`                       |
| Slack rate limit RPS        | `SLACK_RATE_LIMIT_RPS`        | `3.0`                        |
| Slack rate limit burst      | `SLACK_RATE_LIMIT_BURST`      | `10`                         |
| Web enabled                 | `WEB_ENABLED`                 | `false`                      |
| Web listen address          | `WEB_LISTEN_ADDR`             | `:8080`                      |
| Web CORS origins            | `WEB_ALLOWED_ORIGINS`         | `*` (comma-separated)        |
| Storage type                | `STORAGE_TYPE`                | `redis`                      |
| Redis URL                   | `REDIS_URL`                   | `redis://localhost:6379`     |
| Storage TTL                 | `STORAGE_TTL`                 | `604800` (7 days, seconds)   |
| Thread history max threads  | `THREAD_HISTORY_MAX_SIZE`     | `1000`                       |
| Thread history max messages | `THREAD_HISTORY_MAX_MESSAGES` | `50`                         |
| Thread history TTL          | `THREAD_HISTORY_TTL_HOURS`    | `24`                         |

### Config Validation

- `SLACK_BOT_TOKEN` is required when `SLACK_ENABLED=true`
- `SLACK_APP_TOKEN` is required when both `SLACK_ENABLED=true` and `SLACK_SOCKET_MODE=true`

## Client Libraries

### Go Client (`pkg/client/go/`)

`MessagingClient` wraps a gRPC connection with typed helpers:

```go
client, _ := messaging.NewClient("localhost:9090")
defer client.Close()

// Bidirectional stream
stream, _ := client.ProcessConversation(ctx)
stream.SendMessage(messaging.NewMessage("conv-1", "user-1", "alice", "Hello"))

for {
    response, _ := stream.Receive()
    // handle response...
}
stream.Close() // calls CloseSend()
```

**MessagingClient methods**: `NewClient(addr)`, `Close()`, `ProcessConversation(ctx)`, `ProcessMessage(ctx, msg)`, `GetThreadHistory(ctx, convID, maxMsgs)`, `GetConversationMetadata(ctx, convID)`, `HealthCheck(ctx)`

Note: `GetThreadHistory` hardcodes `IncludeEdited: true, IncludeDeleted: false`.

**ConversationStream methods**: `Send(req)`, `SendMessage(msg)`, `SendFeedback(feedback)`, `Receive()`, `ReceiveAll(handler)`, `Close()`

**MessageStream methods**: `Receive()`, `ReceiveAll(handler)` (no Close — server-streaming only)

**Helper constructors**:
- `NewMessage(conversationID, userID, username, content)` — builds `pb.Message` with UUID, timestamp, platform="sdk"
- `NewStatusResponse(conversationID, status, message)` — wraps `StatusUpdate`
- `NewContentResponse(conversationID, content, final)` — `final=true` uses `ContentChunk_END`, else `ContentChunk_START` (not DELTA)
- `NewErrorResponse(conversationID, code, message)` — wraps `ErrorResponse`

### TypeScript Client (`src/messaging-client.ts`)

Uses `@grpc/proto-loader` with `oneofs: true` (flattens oneof fields into plain properties):

```typescript
const client = new MessagingClient("localhost:9090");
await client.connect();
const stream = client.createConversationStream();

stream.on("response", (response) => {
    if (response.incomingMessage) { /* platform message */ }
    if (response.content) { /* content chunk */ }
});

stream.sendAgentResponse(Helpers.createContentResponse(convId, "Hello!"));
```

**ConversationStream methods**: `sendMessage(msg)`, `sendFeedback(feedback)`, `sendAgentConfig(config)`, `sendAgentResponse(response)`, `sendContentChunk(convID, chunk)`, `sendStatusUpdate(convID, status)`, `end()`

Events: `response`, `end`, `error`

**Helpers factory functions**:
- `Helpers.createMessage(convID, userID, username, content)` — hardcodes `platform: 'slack'` (known limitation)
- `Helpers.createStatusResponse(convID, status, message?)`
- `Helpers.createContentResponse(convID, content, final?)`
- `Helpers.createSuggestedPromptsResponse(convID, prompts)`
- `Helpers.createErrorResponse(convID, code, message)`

## Build & Dependencies

Go 1.24.0. Key dependencies:

| Package                        | Version  | Purpose                                |
| ------------------------------ | -------- | -------------------------------------- |
| `google.golang.org/grpc`       | v1.78.0  | gRPC framework                         |
| `google.golang.org/protobuf`   | v1.36.11 | Protobuf runtime                       |
| `github.com/slack-go/slack`    | v0.12.3  | Slack API + Socket Mode                |
| `github.com/google/uuid`       | v1.6.0   | UUID generation                        |
| `github.com/redis/go-redis/v9` | v9.17.2  | Redis client                           |
| `github.com/gorilla/websocket` | v1.5.0   | WebSocket (indirect, used by slack-go) |

### Version Info

`internal/version/version.go` exposes `Version`, `Commit`, `BuildDate` variables set via ldflags at build time. Defaults: `"dev"`, `"unknown"`, `"unknown"`. `Info()` returns a formatted string like `"astro-messaging 0.1.0 (commit: abc123, built: 2025-01-01)"`.

## Key Design Decisions

1. **Sidecar deployment**: The messaging service runs alongside the agent container. The sidecar is the gRPC server; the agent is the client.

2. **Bidirectional stream inversion**: Platform messages are pushed TO the agent by wrapping them as `AgentResponse{IncomingMessage: msg}`. This reuses the server→client stream direction.

3. **No content streaming on Slack**: Slack's 3-second rate limit makes token-by-token streaming a poor UX. Status updates show progress; a single complete message posts on `ContentChunk_END`.

4. **Full content streaming on Web**: The web adapter broadcasts every chunk as an SSE event for real-time streaming in the browser.

5. **Platform thread as ground truth**: `ThreadHistoryStore` stores what's actually in the platform thread including edits and deletions. The agent's AI context is separate and owned by the agent. Thread history is hydrated on demand with 5-minute freshness.

6. **Single unified interface**: All adapters implement one `Adapter` interface with protobuf types throughout. No secondary message format or bridge code.

7. **Pluggable session management**: The web adapter accepts any `SessionManager` implementation. Default is no-op (auth at API gateway level).

8. **Agent owns all context**: The sidecar stores only routing metadata (conversation→platform mapping) and a cache of recent platform thread messages. The agent is responsible for AI context, RAG, memory, and conversation history.

## Test Coverage

120+ tests across all packages:

| Package                  | Tests | Coverage                                                              |
| ------------------------ | ----- | --------------------------------------------------------------------- |
| `internal/adapter/slack` | 50+   | AI features, content buffering, rate limiting, parsing                |
| `internal/adapter/web`   | 40+   | SSE events, handlers, connection management, sessions                 |
| `internal/grpc`          | 30+   | Message routing, conversation cache, health checks, stream management |
| `internal/store`         | 20+   | Thread history CRUD, eviction, staleness, agent config                |
| `pkg/gen/.../v1`         | 6     | Protobuf serialization roundtrips, TypeScript compatibility           |

All tests pass with `-race` flag. `go vet` clean.

## Known Limitations

1. **Single-agent routing**: `HandleIncomingMessage` picks the first stream from a Go map (non-deterministic). Multi-agent routing (conversation→agent affinity) is not implemented.
2. **ProcessMessage stub**: The unary `ProcessMessage` RPC logs and returns nil. All production message flow uses `ProcessConversation` bidi streams.
3. **HealthCheck version**: Returns hardcoded `"1.0.0"` instead of reading from the `version` package.
4. **TypeScript client platform**: `Helpers.createMessage()` hardcodes `platform: 'slack'` regardless of actual platform.
5. **SSE backpressure**: When a client's event channel is full (100 events), new events are silently dropped. No retry or buffering mechanism.
6. **Thread history is in-memory**: `ThreadHistoryStore` has no persistence. Thread data is lost on sidecar restart and re-hydrated on demand.
7. **No TLS on gRPC**: The sidecar-to-agent connection has no TLS. Assumes pod-local or trusted network.
