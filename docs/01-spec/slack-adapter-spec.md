# Slack Adapter Specification

**Version**: 3.0
**Status**: Implemented
**Author**: Astro Team
**Date**: 2026-02-18

## Overview

This document specifies the Slack adapter for Astro Messaging Service. The adapter serves as a thin protocol translation layer between Slack's Socket Mode and the gRPC messaging service, with native support for Slack AI features.

## Goals

1. **Protocol Translation**: Translate Slack events ↔ gRPC proto messages
2. **Socket Mode**: Use Slack's Socket Mode for webhook-free operation
3. **AI Features**: Support Slack AI capabilities (status updates, suggested prompts)
4. **Real-time gRPC**: Bidirectional streaming for instant agent communication
5. **Rate Limiting**: Respect Slack's API rate limits with token bucket throttling
6. **Thin Adapter**: No business logic, pure platform translation

## Non-Goals

1. **AI Conversation Context**: Agent owns semantic context, RAG, memory
2. **Content Streaming**: No incremental message updates (Slack 3s limit makes UX poor)
3. **Block Kit Builder**: Simple messages initially (complex layouts future enhancement)
4. **Slash Commands**: Message-based only (can be extended)
5. **Business Logic**: Adapter doesn't make AI/routing decisions

## Key Design Principle: Thread History Storage

**Critical Insight**: Users can edit and delete messages in Slack. The sidecar MUST store the platform thread history to provide accurate context to the agent.

```
Problem: User edits "What's the weather?" to "What's the weather in Paris?"
Without thread storage: Agent sees old message → Wrong answer ❌
With thread storage: Agent sees edited message → Correct answer ✅
```

**Responsibility Split**:
- **Sidecar stores**: Platform thread messages (what Slack says is in the thread) via shared `ThreadHistoryStore`
- **Agent stores**: AI conversation context (RAG results, memory, session state)
- **Agent requests**: Thread hydration via `GetThreadHistory()` RPC when needed

---

## Architecture

### High-Level Flow

```
┌──────────────────────────────────────────────────┐
│         Slack Platform                           │
│         (WebSocket/Socket Mode)                  │
└──────────────────┬───────────────────────────────┘
                   │ WebSocket Events
                   │ (message, app_mention)
                   ▼
┌──────────────────────────────────────────────────┐
│     SlackAdapter (Messaging Service Sidecar)     │
│  ┌────────────────────────────────────────────┐  │
│  │  Socket Mode Client                        │  │
│  │  - WebSocket connection to Slack           │  │
│  │  - Real-time event delivery                │  │
│  │  - Auto-reconnect on failure               │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Event Handlers                            │  │
│  │  - message.im (DMs)                        │  │
│  │  - app_mention                             │  │
│  │  - Interactive (block_actions for feedback) │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Inline Proto Translation                  │  │
│  │  - Slack Event → pb.Message (inline)       │  │
│  │  - pb.AgentResponse → Slack API            │  │
│  │  - Mention stripping & formatting          │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  AI Features Handler (ai_features.go)      │  │
│  │  - StatusUpdate → setStatus API            │  │
│  │  - ContentChunk → buffer + postMessage     │  │
│  │  - SuggestedPrompts → setSuggestedPrompts  │  │
│  │  - ErrorResponse → postMessage with emoji  │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Rate Limiter (Token Bucket)               │  │
│  │  - 3.0 RPS default, burst 10              │  │
│  │  - 100ms polling interval                  │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  SlackAIClient (Custom HTTP)               │  │
│  │  - assistant.threads.setStatus             │  │
│  │  - assistant.threads.setSuggestedPrompts   │  │
│  │  - assistant.threads.setTitle              │  │
│  │  - chat.postMessage with feedback buttons  │  │
│  └────────────────────────────────────────────┘  │
└──────────────────┬───────────────────────────────┘
                   │ MessageHandler callback
                   │ (grpcServer.HandleIncomingMessage)
                   ▼
┌──────────────────────────────────────────────────┐
│     gRPC Server (Sidecar)                        │
│     - Routes messages to agent stream            │
│     - Routes agent responses to adapter          │
└──────────────────┬───────────────────────────────┘
                   │ Bidirectional gRPC Stream
                   │ ProcessConversation(stream)
                   ▼
┌──────────────────────────────────────────────────┐
│     Agent Container (gRPC Client)                │
│     - Owns conversation context/history          │
│     - Processes messages with AI/LLM             │
│     - Streams responses (status, content, etc.)  │
└──────────────────────────────────────────────────┘
```

### Adapter Capabilities

The Slack adapter declares its capabilities via `adapter.SlackCapabilities()`:

```go
AdapterCapabilities{
    SupportsStreaming:        false,  // No content streaming (3s rate limit)
    SupportsStatusUpdates:    false,  // Hardcoded false (AI client always active regardless)
    SupportsSuggestedPrompts: false,  // Hardcoded false (AI client always active regardless)
    SupportsThreads:          true,   // Native threading
    SupportsTypingIndicator:  false,  // Not available for AI assistants
    MaxUpdateRateHz:          0.33,   // 3 seconds minimum between updates
    MaxContentLength:         4000,   // Slack message limit
    SupportsReactions:        true,   // Emoji reactions
    SupportsCards:            true,   // Rich card attachments
}
```

**Note**: `SupportsStatusUpdates` and `SupportsSuggestedPrompts` are currently hardcoded `false` in the capabilities declaration, but the `SlackAIClient` is always initialized and these features work regardless. The capabilities struct is informational only — nothing gates behavior on it.

---

## Slack Concepts

### Socket Mode

**Socket Mode** is a WebSocket-based connection method that eliminates the need for public webhook URLs:

- **Benefits**:
  - No public endpoint required
  - Works behind firewalls
  - Ideal for local development
  - Real-time event delivery
  - Built-in reconnection logic

- **Requirements**:
  - App-level token with `connections:write` scope
  - Bot token with appropriate scopes
  - Socket Mode enabled in app settings

### Slack AI Features

Slack provides dedicated APIs for AI-powered apps ([Slack AI Documentation](https://docs.slack.dev/ai/developing-ai-apps)):

**assistant.threads.setStatus**:
- Display processing indicators to users
- Cycle through status messages automatically
- Examples: "Thinking...", "Searching knowledge base...", "Generating response..."
- **Rate Limit**: Update no more than every 3 seconds

**assistant.threads.setSuggestedPrompts**:
- Show suggested prompts users can click
- Pre-configured conversation starters
- Max 4-6 prompts per update
- Helps guide user interactions

**assistant_thread_started**:
- Event fired when user opens AI assistant container
- Opportunity to set initial suggested prompts
- Side-by-side interface in Slack client
- **Not yet implemented** in the adapter

**Key Constraints**:
- **3 second minimum** between updates (critical for UX)
- Paid Slack plans required for AI features in production
- Workspace guests cannot access AI apps
- No data storage - retrieve context in real-time

**Why No Content Streaming**: Slack's 3-second rate limit makes incremental updates feel sluggish. Better UX is to show status updates while generating, then send complete message.

### Event Types

Slack sends various event types via Socket Mode:

| Event Type                         | Description         | When Fired                        | Implemented |
| ---------------------------------- | ------------------- | --------------------------------- | ----------- |
| `message.im`                       | Direct message      | User sends DM to bot              | Yes         |
| `app_mention`                      | Bot mentioned       | User @mentions bot in any channel | Yes         |
| `block_actions`                    | Button clicked      | User clicks feedback button       | Yes         |
| `assistant_thread_started`         | AI assistant opened | User opens AI assistant container | No          |
| `assistant_thread_context_changed` | Context switched    | User changes channel context      | No          |
| `message_changed`                  | Message edited      | User edits their message          | No          |
| `message_deleted`                  | Message deleted     | User deletes their message        | No          |
| `reaction_added`                   | Reaction to message | User reacts to bot message        | No          |
| `reaction_removed`                 | Reaction removed    | User removes reaction             | No          |

**Note**: `message.channels` and `message.groups` are intentionally not used to avoid duplicate processing. Use `app_mention` for channel messages.

### Message Threading

Slack uses `thread_ts` (thread timestamp) for threading:

- **Main message**: `ts` field (e.g., `1234567890.123456`)
- **Reply in thread**: `thread_ts` matches parent message `ts`
- **Thread broadcast**: Message appears in channel and thread

### Rate Limits

Slack enforces tiered rate limits:

| Tier   | Rate Limit  | Description             |
| ------ | ----------- | ----------------------- |
| Tier 1 | 1 req/sec   | Basic apps              |
| Tier 2 | 3 req/sec   | Standard apps (default) |
| Tier 3 | 20 req/sec  | Approved apps           |
| Tier 4 | 100 req/sec | Enterprise apps         |

**Default Implementation**: Tier 2 (3 req/sec) with burst of 10

---

## Implementation

### File Structure

```
internal/adapter/slack/
├── adapter.go          # SlackAdapter: lifecycle, event dispatch, message handling
├── ai_features.go      # HandleAgentResponse routing, content buffering
├── slack_ai_api.go     # Custom HTTP client for Slack AI APIs
└── rate_limiter.go     # Token bucket rate limiter

internal/adapter/
├── adapter.go          # Adapter interface (shared across platforms)
└── capabilities.go     # SlackCapabilities(), WebCapabilities()

internal/store/
└── thread_history.go   # ThreadHistoryStore (shared, not Slack-specific)
```

### Core Types

#### SlackAdapter Struct

```go
// internal/adapter/slack/adapter.go

type SlackAdapter struct {
    client         *slack.Client         // Slack REST API client
    socketClient   *socketmode.Client    // Socket Mode WebSocket client
    config         adapter.Config        // Bot token, app token, socket mode, etc.
    msgHandler     adapter.MessageHandler // Callback to gRPC server
    rateLimiter    *RateLimiter          // Token bucket (3.0 RPS, burst 10)
    aiClient       *SlackAIClient        // Custom HTTP client for AI APIs
    contentBuffers map[string]string     // conversationID → accumulated content
    stopChan       chan struct{}         // Shutdown signal
}
```

#### Key Methods

```go
// Adapter interface implementation
func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error
func (a *SlackAdapter) Start(ctx context.Context) error
func (a *SlackAdapter) Stop(ctx context.Context) error
func (a *SlackAdapter) SetMessageHandler(handler adapter.MessageHandler)
func (a *SlackAdapter) GetPlatformName() string        // returns "slack"
func (a *SlackAdapter) IsHealthy(ctx context.Context) bool
func (a *SlackAdapter) Capabilities() adapter.AdapterCapabilities
func (a *SlackAdapter) HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error
func (a *SlackAdapter) HydrateThread(ctx context.Context, conversationID string, store *store.ThreadHistoryStore) error

// Internal event handling
func (a *SlackAdapter) handleSocketEvent(ctx context.Context, evt socketmode.Event)
func (a *SlackAdapter) handleInnerEvent(ctx context.Context, innerEvent slackevents.EventsAPIInnerEvent)
func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent)
func (a *SlackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent)
func (a *SlackAdapter) handleBlockActions(ctx context.Context, callback *slack.InteractionCallback)
func (a *SlackAdapter) sendErrorMessage(ctx context.Context, channelID, threadTS string, err error)

// Helpers (package-level)
func FormatMessageID(channelID, timestamp string) string   // "C123:1234567890.123456"
func ParseMessageID(messageID string) (string, string)     // channelID, timestamp
func stripMentions(text string) string                     // removes <@USERID> patterns
func parseSlackTimestamp(ts string) time.Time
```

---

## Detailed Implementation

### 1. Initialization

**Purpose**: Set up Slack API clients and rate limiter

```go
func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error {
    // Create API client
    a.client = slack.New(config.BotToken, slack.OptionAppLevelToken(config.AppToken))

    // Create Socket Mode client
    a.socketClient = socketmode.New(a.client, socketmode.OptionAppToken(config.AppToken))

    // Create rate limiter (token bucket)
    a.rateLimiter = NewRateLimiter(config.RateLimit.RequestsPerSecond, config.RateLimit.BurstSize)

    // Create AI client (always initialized)
    a.aiClient = NewSlackAIClient(config.BotToken)

    // Initialize content buffers
    a.contentBuffers = make(map[string]string)

    return nil
}
```

### 2. Socket Mode Connection

**Purpose**: Establish and maintain WebSocket connection to Slack

```go
func (a *SlackAdapter) Start(ctx context.Context) error {
    go func() {
        for evt := range a.socketClient.Events {
            a.handleSocketEvent(ctx, evt)
        }
    }()

    // Run socket client (blocking, auto-reconnects)
    return a.socketClient.RunContext(ctx)
}
```

### 3. Event Routing

**Purpose**: Route Socket Mode events to appropriate handlers

```go
func (a *SlackAdapter) handleSocketEvent(ctx context.Context, evt socketmode.Event) {
    switch evt.Type {
    case socketmode.EventTypeEventsAPI:
        a.socketClient.Ack(*evt.Request)
        eventsAPIEvent := evt.Data.(slackevents.EventsAPIEvent)
        a.handleInnerEvent(ctx, eventsAPIEvent.InnerEvent)

    case socketmode.EventTypeInteractive:
        a.socketClient.Ack(*evt.Request)
        callback := evt.Data.(slack.InteractionCallback)
        a.handleBlockActions(ctx, &callback)

    case socketmode.EventTypeConnecting:
        log.Println("[Slack] Connecting...")
    case socketmode.EventTypeConnected:
        log.Println("[Slack] Connected via Socket Mode")
    case socketmode.EventTypeConnectionError:
        log.Printf("[Slack] Connection error: %v", evt.Data)
    }
}
```

### 4. Message Handling

**Purpose**: Process incoming messages and app mentions, build `pb.Message` inline

```go
func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
    // Filter: only DMs (channels starting with 'D')
    // Channel messages handled by app_mention to avoid duplicates
    if ev.Channel != "" && ev.Channel[0] != 'D' {
        return
    }

    // Build conversation ID (just channel for DMs)
    conversationID := ev.Channel

    // Build pb.Message directly (no separate translator)
    msg := &pb.Message{
        Id:             uuid.NewString(),
        Timestamp:      timestamppb.New(parseSlackTimestamp(ev.TimeStamp)),
        Platform:       "slack",
        Content:        ev.Text,
        ConversationId: conversationID,
        PlatformContext: &pb.PlatformContext{
            MessageId: ev.TimeStamp,
            ChannelId: ev.Channel,
            ThreadId:  ev.ThreadTimeStamp,
        },
        User: &pb.User{Id: ev.User},
    }

    // Forward to gRPC server via callback
    a.msgHandler(ctx, msg)
}

func (a *SlackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
    // Strip <@USERID> mentions from text
    text := stripMentions(ev.Text)

    // Thread ID: use existing thread or start new one from message timestamp
    threadID := ev.ThreadTimeStamp
    if threadID == "" {
        threadID = ev.TimeStamp
    }

    // Conversation ID: channelID-threadTS for threaded messages
    conversationID := fmt.Sprintf("%s-%s", ev.Channel, threadID)

    // Set "thinking" status immediately
    a.aiClient.SetThreadStatus(ctx, ev.Channel, threadID, "Thinking...", ":hourglass:")

    // Build pb.Message directly
    msg := &pb.Message{
        Id:             uuid.NewString(),
        Timestamp:      timestamppb.New(parseSlackTimestamp(ev.TimeStamp)),
        Platform:       "slack",
        Content:        text,
        ConversationId: conversationID,
        PlatformContext: &pb.PlatformContext{
            MessageId: ev.TimeStamp,
            ChannelId: ev.Channel,
            ThreadId:  threadID,
        },
        User: &pb.User{Id: ev.User},
    }

    // Forward to gRPC server
    a.msgHandler(ctx, msg)
}
```

### 5. Agent Response Handling (`ai_features.go`)

**Purpose**: Route agent responses to appropriate Slack API calls

```go
func (a *SlackAdapter) HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error {
    switch payload := response.Payload.(type) {
    case *pb.AgentResponse_Status:
        return a.setSlackStatus(ctx, response.ConversationId, payload.Status)
    case *pb.AgentResponse_Content:
        return a.handleContentChunk(ctx, response.ConversationId, payload.Content)
    case *pb.AgentResponse_Prompts:
        return a.setSlackPrompts(ctx, response.ConversationId, payload.Prompts)
    case *pb.AgentResponse_ThreadMetadata:
        return a.handleThreadMetadata(ctx, payload.ThreadMetadata)
    case *pb.AgentResponse_Error:
        return a.handleError(ctx, response.ConversationId, payload.Error)
    }
    return nil
}
```

### 6. Content Buffering Strategy

Slack's 3-second API rate limit makes token-by-token streaming a poor UX. The adapter buffers content chunks and sends one complete message:

```go
func (a *SlackAdapter) handleContentChunk(ctx context.Context, conversationID string, content *pb.ContentChunk) error {
    channelID, threadTS, _ := parseConversationID(conversationID)

    switch content.Type {
    case pb.ContentChunk_START:
        // Reset buffer for new response
        a.contentBuffers[conversationID] = ""

    case pb.ContentChunk_DELTA:
        // Accumulate content
        a.contentBuffers[conversationID] += content.Content

    case pb.ContentChunk_END:
        // Flush: post complete buffered message
        fullContent := a.contentBuffers[conversationID]
        delete(a.contentBuffers, conversationID)

        // Post via feedback-enabled message or plain message
        a.aiClient.PostMessageWithFeedback(ctx, channelID, fullContent, threadTS)

    case pb.ContentChunk_REPLACE:
        // Send immediately without buffering
        a.client.PostMessageContext(ctx, channelID,
            slack.MsgOptionText(content.Content, false),
            slack.MsgOptionTS(threadTS),
        )
    }
    return nil
}
```

### 7. SlackAIClient (`slack_ai_api.go`)

Custom HTTP client for Slack AI APIs not available in the `slack-go` library:

```go
type SlackAIClient struct {
    botToken   string
    httpClient *http.Client
    baseURL    string  // "https://slack.com/api"
}

type SuggestedPrompt struct {
    Title   string `json:"title"`
    Message string `json:"message"`
}
```

Methods:
- `SetThreadStatus(ctx, channelID, threadTS, status, emoji)` → `assistant.threads.setStatus`
- `SetSuggestedPrompts(ctx, channelID, threadTS, prompts)` → `assistant.threads.setSuggestedPrompts`
- `SetTitle(ctx, channelID, threadTS, title)` → `assistant.threads.setTitle`
- `PostMessageWithFeedback(ctx, channelID, content, threadID)` → `chat.postMessage` with Block Kit

`PostMessageWithFeedback` sends two Block Kit blocks:
1. `section` block with `mrkdwn` text containing the message content
2. `context_actions` block with `feedback_buttons` action containing thumbs up (`positive_feedback`) and thumbs down (`negative_feedback`) buttons

### 8. Feedback Button Handling

When a user clicks a feedback button, `handleBlockActions()`:
1. Removes the feedback button blocks from the message (via `chat.update`)
2. Adds a reaction emoji to the message (thumbs up or thumbs down via Slack reactions API)

### 9. Rate Limiter

Token bucket implementation (not adaptive/debouncing as originally spec'd):

```go
type RateLimiter struct {
    tokens     float64
    maxTokens  float64     // = burstSize (starts full)
    refillRate float64     // tokens per second
    lastRefill time.Time
    mu         sync.Mutex
}
```

- `Wait(ctx)` — polls every 100ms, calls `refill()`, blocks until token available or context cancelled
- `TryAcquire()` — non-blocking, returns true if token consumed
- `refill()` — computes `elapsed * refillRate`, clamps at `maxTokens`

Default: 3.0 RPS, burst 10.

### 10. Thread Hydration

`HydrateThread()` fetches messages from Slack and stores them in the shared `ThreadHistoryStore`:

```go
func (a *SlackAdapter) HydrateThread(ctx context.Context, conversationID string, threadStore *store.ThreadHistoryStore) error {
    // Parse "channelID-threadTS" conversation ID
    channelID, threadTS := parseConversationParts(conversationID)

    // Fetch from Slack API
    if threadTS != "" {
        // Threaded: GetConversationReplies
        msgs, _, _, _ := a.client.GetConversationRepliesContext(ctx, ...)
    } else {
        // DM: GetConversationHistory
        history, _ := a.client.GetConversationHistoryContext(ctx, ...)
    }

    // Convert to pb.ThreadMessage and store
    for _, msg := range messages {
        threadStore.AddMessage(conversationID, &pb.ThreadMessage{...})
    }

    return nil
}
```

Called by the gRPC server when `ThreadHistoryStore.IsStale(conversationID, 5*time.Minute)` returns true.

### 11. Conversation ID Format

- **DMs**: `channelID` (e.g., `D1234567`)
- **Channel threads**: `channelID-threadTS` (e.g., `C1234567-1234567890.000001`)

### 12. Message ID Format

`FormatMessageID` produces `channelID:timestamp` (e.g., `C123456:1234567890.123456`).
`ParseMessageID` splits on `:` — returns empty strings for invalid format.

---

## Configuration

### Environment Variables

```bash
# Slack Adapter
SLACK_ENABLED=true
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_APP_TOKEN=xapp-your-app-token-here
SLACK_SOCKET_MODE=true
SLACK_AUTO_THREAD=true
SLACK_RATE_LIMIT_RPS=3.0
SLACK_RATE_LIMIT_BURST=10
```

### Config Validation

- `SLACK_BOT_TOKEN` is required when `SLACK_ENABLED=true`
- `SLACK_APP_TOKEN` is required when both `SLACK_ENABLED=true` and `SLACK_SOCKET_MODE=true`

---

## Slack App Setup

### Step-by-Step Guide

#### 1. Create Slack App

1. Go to https://api.slack.com/apps
2. Click "Create New App"
3. Choose "From scratch"
4. Enter app name and select workspace
5. Click "Create App"

#### 2. Enable Socket Mode

1. Navigate to "Socket Mode" in sidebar
2. Toggle "Enable Socket Mode" to ON
3. Click "Generate Token"
4. Name: "Socket Token"
5. Scope: `connections:write`
6. Copy the token (starts with `xapp-`)

#### 3. Add Bot Token Scopes

1. Navigate to "OAuth & Permissions"
2. Scroll to "Scopes" → "Bot Token Scopes"
3. Add the following scopes:
   - `chat:write` - Send messages
   - `im:history` - Read DM messages
   - `app_mentions:read` - Receive @mentions
   - `reactions:read` - Read message reactions (feedback)
   - `reactions:write` - Add reaction emoji (feedback response)
   - `channels:history` - Read channel messages (for thread hydration)

   **For AI Features** (requires paid plan):
   - `assistant:write` - Use AI assistant features
   - `assistant:read` - Read assistant events

#### 4. Subscribe to Bot Events

1. Navigate to "Event Subscriptions"
2. Scroll to "Subscribe to bot events"
3. Add the following events:
   - `message.im` - Direct messages
   - `app_mention` - @mentions of the bot

   **For AI Features**:
   - `assistant_thread_started` - AI assistant opened
   - `assistant_thread_context_changed` - Context changed

4. Click "Save Changes"

#### 5. Enable Interactivity

1. Navigate to "Interactivity & Shortcuts"
2. Toggle "Interactivity" to ON
3. This enables feedback button clicks to be received

#### 6. Enable AI Assistant (Optional)

**Required for AI features**:
1. Navigate to "App Settings" → "AI & Automation"
2. Toggle "Enable AI Assistant" to ON
3. Configure assistant display name
4. Set assistant description (shown to users)
5. **Note**: Requires paid Slack workspace

#### 7. Install App to Workspace

1. Navigate to "OAuth & Permissions"
2. Click "Install to Workspace"
3. Review permissions and click "Allow"
4. Copy the "Bot User OAuth Token" (starts with `xoxb-`)

#### 8. Test Connection

1. For regular bot:
   - Add bot to a channel: `/invite @YourBotName`
   - Send test message: `@YourBotName hello`

2. For AI assistant:
   - Open assistant from top bar
   - Send test message

3. Check messaging logs for connection

---

## Agent Usage Patterns

### When Should Agent Call GetThreadHistory?

**Always hydrate when**:
1. **First message in conversation**: Get full thread context
2. **After edit notification**: Refresh to see latest edits
3. **User references prior context**: "Like I said earlier..."
4. **Long gap between messages**: Thread may have changed (>5 min)

**Optimization**: Cache thread locally in agent, refresh only when needed

**Example Agent Logic**:
```python
async def process_message(self, message: Message):
    should_hydrate = (
        self.is_first_message(message.conversation_id) or
        self.received_edit_notification(message.conversation_id) or
        self.time_since_last_message(message.conversation_id) > 300  # 5 min
    )

    if should_hydrate:
        thread_history = await self.get_thread_history(message.conversation_id)
        context = self.build_context_from_thread(thread_history)
    else:
        context = self.get_cached_context(message.conversation_id)

    response = await self.llm.generate(context, message.content)
    await self.send_response(response)
```

## Message Flows

### Incoming Message Flow (App Mention with AI)

```
User in Slack → Types "@BotName what's the weather?" in #general
    ↓
Slack Platform → Detects app mention
    ↓
Socket Mode → WebSocket event to messaging sidecar
    ↓
SlackAdapter.handleSocketEvent()
    ↓
EventTypeEventsAPI → Ack immediately
    ↓
handleInnerEvent() → app_mention event
    ↓
handleAppMention()
    ↓
stripMentions() + build pb.Message inline
    ↓
aiClient.SetThreadStatus("Thinking...") ← immediate
    ↓
msgHandler() [= grpcServer.HandleIncomingMessage]
    ↓
gRPC Server wraps as AgentResponse{IncomingMessage: msg}
    ↓
stream.Send() to agent
    ↓
Agent receives message, processes with AI/LLM
    ↓
Agent sends AgentResponse{Content: ContentChunk{START}} → buffer reset
Agent sends AgentResponse{Content: ContentChunk{DELTA, "The weather..."}} → buffer append
Agent sends AgentResponse{Content: ContentChunk{END}} → flush
    ↓
gRPC Server → routeAgentResponse() → adapter.HandleAgentResponse()
    ↓
handleContentChunk(END) → PostMessageWithFeedback()
    ↓
User sees complete response with 👍/👎 buttons
```

### No Content Streaming (By Design)

**Unlike other platforms**, Slack does NOT support smooth content streaming due to 3-second rate limit:

```
❌ BAD UX (streaming every token):
Message: "The"        (0s)
Update:  "The weath"  (3s) ← Long delay, feels broken
Update:  "The weather is" (6s) ← Another delay
User Experience: Janky, unresponsive

✅ GOOD UX (status + complete message):
Status: "Thinking..."          (immediate)
Status: "Generating response..."  (after processing)
Message: "The weather is sunny" (complete, when done)
User Experience: Smooth, clear progress
```

---

## Missing Features

Features from the original spec that are **not yet implemented**:

### Event Handling Gaps

| Feature | Spec Section | Impact |
| ------- | ------------ | ------ |
| `assistant_thread_started` handler | Event fired when user opens AI assistant | Cannot set initial suggested prompts on assistant open |
| `assistant_thread_context_changed` handler | Context switch event | Cannot adapt when user switches channel context |
| `message_changed` handler | Message edit detection | Edits not tracked — thread history becomes stale |
| `message_deleted` handler | Message deletion detection | Deleted messages remain in thread history |
| `reaction_added` / `reaction_removed` handler | Emoji reaction feedback | Only button-click feedback works, not emoji reactions |

### Thread History Accuracy

Without `message_changed` and `message_deleted` handlers, thread history stored in the sidecar can become inconsistent with what's actually in Slack. The `HydrateThread()` refresh (triggered when history is >5 min stale) compensates partially, but real-time accuracy requires implementing these event handlers and sending `PlatformFeedback` to the agent.

### Agent Notification of Edits/Deletes

The spec designed `PlatformFeedback` messages (`MessageEdit`, `MessageDelete`) to notify agents when users edit or delete messages. These proto types exist but are never sent because the edit/delete event handlers are not implemented.

### Capabilities Declaration

`SlackCapabilities(false)` hardcodes `SupportsStatusUpdates` and `SupportsSuggestedPrompts` to `false`. Should accept a parameter or read from config to accurately reflect AI feature availability.

### Additional Missing Features

- **File attachment translation**: Slack file uploads are not converted to `pb.Attachment`
- **User profile enrichment**: `pb.User` only populates `Id`, not `Username`, `Email`, or `AvatarUrl`
- **Block Kit responses**: Only plain text and feedback buttons — no rich card layouts
- **Slash commands**: Not handled (`EventTypeSlashCommand` is ack'd but ignored)
- **Interactive shortcuts**: Not implemented
- **Home tab**: Not implemented
- **Multi-workspace**: Single workspace per adapter instance

---

## Error Handling

### Connection Errors

Socket Mode auto-reconnects on connection errors. Connection events are logged.

### Message Send Errors

```go
func (a *SlackAdapter) sendErrorMessage(ctx context.Context, channelID, threadTS string, err error) {
    a.client.PostMessageContext(ctx, channelID,
        slack.MsgOptionText("Sorry, I encountered an error.", false),
        slack.MsgOptionTS(threadTS),
    )
}
```

### Agent Error Responses

`handleError()` posts the error message with a warning emoji prefix to the conversation thread.

---

## Testing

### Unit Tests

Tests in `internal/adapter/slack/`:

| Test File | What's Tested |
| --------- | ------------- |
| `ai_features_test.go` | HandleAgentResponse routing, content buffering (START/DELTA/END/REPLACE), status mapping, error handling |
| `rate_limiter_test.go` | Token bucket behavior, burst, refill, context cancellation |
| `slack_ai_api_test.go` | HTTP request formatting, suggested prompts, status updates |
| `translator_test.go` | `stripMentions`, `FormatMessageID`, `ParseMessageID` |

### Manual Testing

1. Start messaging service with Slack adapter enabled
2. Start agent with gRPC client connected
3. Send `@BotName hello` in Slack channel
4. Verify:
   - Status update appears ("Thinking...")
   - Bot responds in thread
   - Response has feedback buttons
   - Clicking feedback button adds reaction and removes buttons
5. Send DM to bot, verify response

---

## Deployment

### Docker Compose

```yaml
services:
  messaging:
    ports:
      - "9090:9090"  # gRPC port
    environment:
      - SLACK_ENABLED=true
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - SLACK_SOCKET_MODE=true
      - SLACK_AUTO_THREAD=true
      - GRPC_LISTEN_ADDR=:9090

  agent:
    environment:
      - GRPC_ADDRESS=messaging:9090
    depends_on:
      - messaging
```

### Kubernetes

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: slack-credentials
type: Opaque
data:
  bot-token: <base64-encoded-bot-token>
  app-token: <base64-encoded-app-token>

---
apiVersion: v1
kind: Service
metadata:
  name: astro-messaging
spec:
  selector:
    app: astro-messaging
  ports:
  - name: grpc
    port: 9090
    targetPort: 9090

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: astro-messaging
spec:
  template:
    spec:
      containers:
      - name: messaging
        ports:
        - containerPort: 9090
          name: grpc
        env:
        - name: SLACK_ENABLED
          value: "true"
        - name: SLACK_BOT_TOKEN
          valueFrom:
            secretKeyRef:
              name: slack-credentials
              key: bot-token
        - name: SLACK_APP_TOKEN
          valueFrom:
            secretKeyRef:
              name: slack-credentials
              key: app-token
        - name: GRPC_LISTEN_ADDR
          value: ":9090"
```

---

## Troubleshooting

### Bot Not Responding

**Check logs**:
```bash
docker compose logs messaging | grep -i slack
docker compose logs messaging | grep -i grpc
```

**Common issues**:
- Tokens incorrect (verify in .env)
- Socket Mode not enabled in app settings
- Bot not invited to channel (`/invite @BotName`)
- Event subscriptions not configured
- Agent not connected via gRPC stream

### AI Features Not Working

**Common issues**:
- **Free Slack workspace** (requires paid plan)
- Bot not configured as AI assistant in Slack app settings
- Missing `assistant:write` scope

### Connection Drops

Socket Mode auto-reconnects, but if persistent:

- Check network connectivity
- Verify tokens haven't been revoked
- Check Slack API status page

### Rate Limiting

If seeing "rate_limited" errors from Slack:

- Token bucket defaults to 3.0 RPS with burst 10
- Status updates via AI client bypass the rate limiter (use Slack's own rate limiting)
- Check Slack API tier (default: Tier 2 = 3 req/sec)

---

## References

### Slack
- **Slack API Docs**: https://api.slack.com/
- **Slack AI Apps Guide**: https://docs.slack.dev/ai/developing-ai-apps
- **Socket Mode Guide**: https://api.slack.com/apis/connections/socket
- **Events API**: https://api.slack.com/events-api
- **Rate Limits**: https://api.slack.com/docs/rate-limits
- **Block Kit**: https://api.slack.com/block-kit
- **Go SDK**: https://github.com/slack-go/slack

### gRPC & Protocol Buffers
- **gRPC Go**: https://grpc.io/docs/languages/go/
- **gRPC Streaming**: https://grpc.io/docs/what-is-grpc/core-concepts/#bidirectional-streaming-rpc
- **Protocol Buffers**: https://protobuf.dev/

### Related Docs
- **Architecture**: `docs/03-architecture/astro-messaging.md`

---

## Implementation Status

### Core Functionality
- [x] Socket Mode connection established and maintained
- [x] Receives messages in DMs and via @mentions
- [x] gRPC bidirectional streaming with agent (via sidecar)
- [x] Responds with complete messages (buffered, not streamed)
- [x] Threading works correctly (auto-thread in channels)
- [x] Graceful reconnection on Slack disconnect
- [x] Health checks report connection status
- [x] Feedback buttons on responses (thumbs up/down)

### AI Features
- [x] Status updates via `assistant.threads.setStatus`
- [x] Suggested prompts via `assistant.threads.setSuggestedPrompts`
- [x] Thread title via `assistant.threads.setTitle`
- [ ] `assistant_thread_started` event handling
- [ ] `assistant_thread_context_changed` event handling

### Missing
- [ ] Message edit event handling (`message_changed`)
- [ ] Message delete event handling (`message_deleted`)
- [ ] Reaction feedback (`reaction_added` / `reaction_removed`)
- [ ] File attachment translation
- [ ] User profile enrichment
- [ ] Slash commands
- [ ] Block Kit rich responses
- [ ] Capabilities declaration accuracy (hardcoded false for AI features)
