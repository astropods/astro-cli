# Slack Adapter Specification (gRPC)

**Version**: 2.0
**Status**: Design
**Author**: Astro Team
**Date**: 2026-01-21

## Overview

This document specifies the gRPC-based Slack adapter for Astro Messaging Service. The adapter serves as a thin protocol translation layer between Slack's Socket Mode and the gRPC messaging service, with native support for Slack AI features.

## Goals

1. **Protocol Translation**: Translate Slack events ↔ gRPC proto messages
2. **Socket Mode**: Use Slack's Socket Mode for webhook-free operation
3. **AI Features**: Support Slack AI capabilities (status updates, suggested prompts)
4. **Real-time gRPC**: Bidirectional streaming for instant agent communication
5. **Rate Limiting**: Respect Slack's API rate limits with adaptive throttling
6. **Thin Adapter**: No business logic, pure platform translation

## Non-Goals

1. **AI Conversation Context**: Agent owns semantic context, RAG, memory
2. **Content Streaming**: No incremental message updates (Slack 3s limit makes UX poor)
3. **Block Kit Builder**: Simple messages initially (complex layouts future enhancement)
4. **Slash Commands**: Message-based only (can be extended)
5. **Business Logic**: Adapter doesn't make AI/routing decisions

## Key Design Principle: Thread History Storage

**Critical Insight**: Users can edit and delete messages in Slack. The adapter MUST store the platform thread history to provide accurate context to the agent.

```
Problem: User edits "What's the weather?" to "What's the weather in Paris?"
Without thread storage: Agent sees old message → Wrong answer ❌
With thread storage: Agent sees edited message → Correct answer ✅
```

**Responsibility Split**:
- **Adapter stores**: Platform thread messages (what Slack says is in the thread)
- **Agent stores**: AI conversation context (RAG results, memory, session state)
- **Agent requests**: Thread hydration via `GetThreadHistory()` when needed

---

## Architecture

### High-Level Flow

```
┌──────────────────────────────────────────────────┐
│         Slack Platform                           │
│         (WebSocket/Socket Mode)                  │
└──────────────────┬───────────────────────────────┘
                   │ WebSocket Events
                   │ (message, app_mention, reactions)
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
│  │  - assistant_thread_started (AI)          │  │
│  │  - message reactions (feedback)            │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Proto Translator                          │  │
│  │  - Slack Event → proto.Message             │  │
│  │  - proto.AgentResponse → Slack API         │  │
│  │  - Mention stripping & formatting          │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  AI Features Handler                       │  │
│  │  - StatusUpdate → setStatus API            │  │
│  │  - SuggestedPrompts → setSuggestedPrompts  │  │
│  │  - ThreadMetadata → thread management      │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Rate Limiter (Adaptive)                   │  │
│  │  - 0.33 Hz (3 second minimum per Slack)    │  │
│  │  - Debouncing for status updates           │  │
│  └────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────┐  │
│  │  Slack API Client                          │  │
│  │  - chat.postMessage                        │  │
│  │  - assistant.threads.setStatus (AI)        │  │
│  │  - assistant.threads.setSuggestedPrompts   │  │
│  └────────────────────────────────────────────┘  │
└──────────────────┬───────────────────────────────┘
                   │ gRPC Bidirectional Stream
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

The Slack adapter declares its capabilities for the messaging service:

```go
AdapterCapabilities{
    SupportsStreaming:        false,  // No content streaming (3s rate limit)
    SupportsStatusUpdates:    true,   // assistant.threads.setStatus
    SupportsSuggestedPrompts: true,   // assistant.threads.setSuggestedPrompts
    SupportsThreads:          true,   // Native threading
    SupportsTypingIndicator:  false,  // Not available for AI assistants
    MaxUpdateRateHz:          0.33,   // 3 seconds minimum between updates
    MaxContentLength:         4000,   // Slack message limit
}
```

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

**Key Constraints**:
- **3 second minimum** between updates (critical for UX)
- Paid Slack plans required for AI features in production
- Workspace guests cannot access AI apps
- No data storage - retrieve context in real-time

**Why No Content Streaming**: Slack's 3-second rate limit makes incremental updates feel sluggish. Better UX is to show status updates while generating, then send complete message.

### Event Types

Slack sends various event types via Socket Mode:

| Event Type                    | Description                  | When Fired                                | AI Feature |
| ----------------------------- | ---------------------------- | ----------------------------------------- | ---------- |
| `message.im`                  | Direct message               | User sends DM to bot                      | No         |
| `app_mention`                 | Bot mentioned                | User @mentions bot in any channel         | No         |
| `assistant_thread_started`    | AI assistant opened          | User opens AI assistant container         | Yes        |
| `assistant_thread_context_changed` | Context switched       | User changes channel context              | Yes        |
| `message_changed`             | Message edited               | User edits their message                  | No         |
| `message_deleted`             | Message deleted              | User deletes their message                | No         |
| `reaction_added`              | Reaction to message          | User reacts to bot message                | Feedback   |
| `reaction_removed`            | Reaction removed             | User removes reaction                     | Feedback   |

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

**Default Implementation**: Tier 2 (3 req/sec) with burst support

---

## Implementation

### File Structure

```
internal/adapter/slack/
├── adapter.go          # SlackAdapter implementation (new interface)
├── translator.go       # Event translation Slack ↔ proto
├── ai_features.go      # Slack AI features (setStatus, setSuggestedPrompts)
├── thread_history.go   # Thread storage & hydration from Slack API
├── edit_handler.go     # Message edit/delete event handling
├── rate_limiter.go     # Adaptive rate limiter (3s debouncing)
└── capabilities.go     # Adapter capabilities declaration
```

### Core Types

#### SlackAdapter Struct

```go
// internal/adapter/slack/adapter.go

type SlackAdapter struct {
    // Slack API clients
    api          *slack.Client        // REST API client
    socketClient *socketmode.Client   // Socket Mode WebSocket client

    // Configuration
    botToken     string
    appToken     string
    aiFeatures   bool                 // Enable Slack AI features
    autoThread   bool

    // gRPC stream to agent
    agentStream  grpc.ClientStream

    // Rate limiting (3 second minimum for Slack)
    rateLimiter *AdaptiveRateLimiter

    // Conversation metadata cache (light LRU for routing)
    conversationCache *ConversationCache

    // Thread history storage (platform messages with edits/deletes)
    threadHistoryStore *ThreadHistoryStore

    // Health status
    connected bool
    mu        sync.RWMutex
}

// Thread history store - ground truth for Slack thread state
type ThreadHistoryStore struct {
    store map[string]*ThreadHistory  // conversation_id -> history
    mu    sync.RWMutex
    ttl   time.Duration
}

type ThreadHistory struct {
    ConversationID string
    Messages       []*ThreadMessage
    LastFetched    time.Time
    MaxMessages    int  // Keep last N messages
}
```

#### Key Methods

```go
func NewSlackAdapter(botToken, appToken string, config SlackConfig) *SlackAdapter

// Adapter interface implementation (new gRPC design)
func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error
func (a *SlackAdapter) Start(ctx context.Context) error
func (a *SlackAdapter) Stop(ctx context.Context) error
func (a *SlackAdapter) OnMessage(handler MessageHandler)
func (a *SlackAdapter) Capabilities() AdapterCapabilities
func (a *SlackAdapter) GetPlatformName() string
func (a *SlackAdapter) IsHealthy(ctx context.Context) bool

// AI features (new)
func (a *SlackAdapter) SetStatus(ctx context.Context, conversationID string, status *pb.StatusUpdate) error
func (a *SlackAdapter) SetSuggestedPrompts(ctx context.Context, conversationID string, prompts *pb.SuggestedPrompts) error
func (a *SlackAdapter) UpdateThreadMetadata(ctx context.Context, metadata *pb.ThreadMetadata) error

// Streaming responses (new)
func (a *SlackAdapter) StreamResponse(ctx context.Context, req *StreamResponseRequest) error

// Thread history hydration (new)
func (a *SlackAdapter) GetThreadHistory(ctx context.Context, req *pb.ThreadHistoryRequest) (*pb.ThreadHistoryResponse, error)
func (a *SlackAdapter) hydrateThreadFromSlack(ctx context.Context, conversationID string) error

// Internal event handling
func (a *SlackAdapter) handleSocketEvent(ctx context.Context, evt socketmode.Event)
func (a *SlackAdapter) handleInnerEvent(ctx context.Context, innerEvent slackevents.EventsAPIInnerEvent)
func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent)
func (a *SlackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent)
func (a *SlackAdapter) handleAssistantThreadStarted(ctx context.Context, ev *slackevents.AssistantThreadStartedEvent)
func (a *SlackAdapter) handleReactionAdded(ctx context.Context, ev *slackevents.ReactionAddedEvent)

// Message edit/delete handling (new)
func (a *SlackAdapter) handleMessageChanged(ctx context.Context, ev *slackevents.MessageEvent)
func (a *SlackAdapter) handleMessageDeleted(ctx context.Context, ev *slackevents.MessageEvent)
```

---

## Detailed Implementation

### 1. Initialization

**Purpose**: Set up Slack API clients, gRPC connection, and capabilities

```go
func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error {
    // Validate tokens
    if a.botToken == "" || a.appToken == "" {
        return fmt.Errorf("Slack bot token and app token required")
    }

    // Create API client (for sending messages)
    a.api = slack.New(
        a.botToken,
        slack.OptionAppLevelToken(a.appToken),
    )

    // Create Socket Mode client (for receiving events)
    a.socketClient = socketmode.New(
        a.api,
        socketmode.OptionAppToken(a.appToken),
    )

    // Create adaptive rate limiter (3 second debouncing)
    a.rateLimiter = NewAdaptiveRateLimiter(0.33) // 3 seconds

    // Initialize conversation cache (LRU, 1000 entries, 1hr TTL)
    a.conversationCache = NewConversationCache(1000, time.Hour)

    log.Println("[Slack] Adapter initialized")
    return nil
}

func (a *SlackAdapter) Capabilities() AdapterCapabilities {
    return AdapterCapabilities{
        SupportsStreaming:        false,  // No content streaming
        SupportsStatusUpdates:    a.aiFeatures,
        SupportsSuggestedPrompts: a.aiFeatures,
        SupportsThreads:          true,
        SupportsTypingIndicator:  false,
        MaxUpdateRateHz:          0.33,   // 3 seconds
        MaxContentLength:         4000,
    }
}
```

### 2. Socket Mode Connection

**Purpose**: Establish and maintain WebSocket connection to Slack

```go
func (a *SlackAdapter) Start(ctx context.Context) error {
    log.Println("[Slack] Starting Socket Mode connection...")

    // Handle socket events in goroutine
    go func() {
        for evt := range a.socketClient.Events {
            a.handleSocketEvent(ctx, evt)
        }
    }()

    // Run socket client (blocking)
    return a.socketClient.RunContext(ctx)
}
```

### 3. Event Routing

**Purpose**: Route Socket Mode events to appropriate handlers

```go
func (a *SlackAdapter) handleSocketEvent(ctx context.Context, evt socketmode.Event) {
    switch evt.Type {
    case socketmode.EventTypeConnecting:
        log.Println("[Slack] Connecting to Slack...")

    case socketmode.EventTypeConnectionError:
        log.Printf("[Slack] Connection error: %v", evt.Data)
        a.setConnected(false)

    case socketmode.EventTypeConnected:
        log.Println("[Slack] Connected to Slack via Socket Mode")
        a.setConnected(true)

    case socketmode.EventTypeDisconnect:
        log.Println("[Slack] Disconnected from Slack")
        a.setConnected(false)

    case socketmode.EventTypeEventsAPI:
        // Acknowledge event immediately
        a.socketClient.Ack(*evt.Request)

        // Parse and handle inner event
        eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
        if !ok {
            log.Printf("[Slack] Could not cast event to EventsAPIEvent")
            return
        }

        a.handleInnerEvent(ctx, eventsAPIEvent.InnerEvent)

    case socketmode.EventTypeHello:
        // Connection acknowledgment, no action needed

    case socketmode.EventTypeInteractive:
        // Interactive components (buttons, menus) - future
        a.socketClient.Ack(*evt.Request)
        log.Println("[Slack] Interactive event (not yet handled)")

    case socketmode.EventTypeSlashCommand:
        // Slash commands - future
        a.socketClient.Ack(*evt.Request)
        log.Println("[Slack] Slash command (not yet handled)")

    default:
        if evt.Type != "" {
            log.Printf("[Slack] Unhandled event type: %s", evt.Type)
        }
    }
}
```

### 4. Message Event Handling

**Purpose**: Process incoming messages and app mentions

```go
func (a *SlackAdapter) handleInnerEvent(ctx context.Context, innerEvent slackevents.EventsAPIInnerEvent) {
    switch ev := innerEvent.Data.(type) {
    case *slackevents.MessageEvent:
        // Route based on subtype
        switch ev.SubType {
        case "message_changed":
            a.handleMessageChanged(ctx, ev)
        case "message_deleted":
            a.handleMessageDeleted(ctx, ev)
        default:
            a.handleMessage(ctx, ev)
        }

    case *slackevents.AppMentionEvent:
        a.handleAppMention(ctx, ev)

    case *slackevents.ReactionAddedEvent:
        a.handleReactionAdded(ctx, ev)

    case *slackevents.ReactionRemovedEvent:
        a.handleReactionRemoved(ctx, ev)

    default:
        log.Printf("[Slack] Unhandled inner event type: %s", innerEvent.Type)
    }
}

func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
    // Filter out bot messages
    if ev.BotID != "" {
        return
    }

    // Filter out message subtypes (except thread_broadcast)
    if ev.SubType != "" && ev.SubType != "thread_broadcast" {
        return
    }

    // Only process DM messages (channel starts with 'D')
    // Channel messages are handled via app_mention to avoid duplicates
    if ev.Channel != "" && ev.Channel[0] != 'D' {
        log.Printf("[Slack] Ignoring channel message (use app_mention)")
        return
    }

    log.Printf("[Slack] Message: channel=%s, user=%s", ev.Channel, ev.User)

    // Translate and process
    unifiedMsg := TranslateMessageEvent(ev)
    a.processMessage(ctx, unifiedMsg)
}

func (a *SlackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
    log.Printf("[Slack] Mention: channel=%s, user=%s", ev.Channel, ev.User)

    // Translate and process
    unifiedMsg := TranslateAppMentionEvent(ev)
    a.processMessage(ctx, unifiedMsg)
}

func (a *SlackAdapter) processMessage(ctx context.Context, protoMsg *pb.Message) {
    // Store message in thread history
    threadMsg := &pb.ThreadMessage{
        MessageId:    protoMsg.PlatformContext.MessageId,
        User:         protoMsg.User,
        Content:      protoMsg.Content,
        Attachments:  protoMsg.Attachments,
        Timestamp:    protoMsg.Timestamp,
        WasEdited:    false,
        IsDeleted:    false,
        PlatformData: protoMsg.PlatformContext.PlatformData,
    }

    a.threadHistoryStore.AddMessage(protoMsg.ConversationId, threadMsg)

    // Send to agent via gRPC stream
    if a.agentStream != nil {
        a.agentStream.Send(&pb.ConversationRequest{
            Request: &pb.ConversationRequest_Message{
                Message: protoMsg,
            },
        })
    }
}
```

### 5. Message Translation (Slack → Proto)

**Purpose**: Convert Slack events to proto.Message format

```go
// internal/adapter/slack/translator.go

func TranslateMessageEvent(ev *slackevents.MessageEvent) *pb.Message {
    // Build conversation ID
    conversationID := ev.Channel
    if ev.ThreadTimeStamp != "" {
        conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
    }

    return &pb.Message{
        Id:        uuid.NewString(),
        Timestamp: timestamppb.New(parseSlackTimestamp(ev.TimeStamp)),
        Platform:  "slack",
        PlatformContext: &pb.PlatformContext{
            MessageId:   ev.TimeStamp,
            ChannelId:   ev.Channel,
            ThreadId:    ptrToString(ev.ThreadTimeStamp),
            PlatformData: map[string]string{
                "event_ts": ev.EventTimeStamp,
                "subtype":  ev.SubType,
            },
        },
        User: &pb.User{
            Id:       ev.User,
            Username: ev.User, // Fetch display name if needed
        },
        Content:        ev.Text,
        Attachments:    translateSlackFiles(ev.Files),
        ConversationId: conversationID,
    }
}

func TranslateAppMentionEvent(ev *slackevents.AppMentionEvent) *pb.Message {
    conversationID := ev.Channel
    if ev.ThreadTimeStamp != "" {
        conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
    }

    // Strip bot mention from text
    text := stripMentions(ev.Text)

    return &pb.Message{
        Id:        uuid.NewString(),
        Timestamp: timestamppb.New(parseSlackTimestamp(ev.TimeStamp)),
        Platform:  "slack",
        PlatformContext: &pb.PlatformContext{
            MessageId: ev.TimeStamp,
            ChannelId: ev.Channel,
            ThreadId:  ptrToString(ev.ThreadTimeStamp),
            PlatformData: map[string]string{
                "event_ts": ev.EventTimeStamp,
            },
        },
        User: &pb.User{
            Id:       ev.User,
            Username: ev.User,
        },
        Content:        text,
        ConversationId: conversationID,
    }
}

// stripMentions removes <@USERID> mentions from message text
func stripMentions(text string) string {
    re := regexp.MustCompile(`<@[A-Z0-9]+>`)
    text = re.ReplaceAllString(text, "")
    return strings.TrimSpace(text)
}

func translateSlackFiles(files []slack.File) []*pb.Attachment {
    var attachments []*pb.Attachment
    for _, f := range files {
        attachments = append(attachments, &pb.Attachment{
            Type:      determineAttachmentType(f.Mimetype),
            Url:       f.URLPrivateDownload,
            Filename:  f.Name,
            SizeBytes: &f.Size,
            MimeType:  &f.Mimetype,
        })
    }
    return attachments
}
```

### 6. Streaming Agent Responses (Proto → Slack)

**Purpose**: Handle streamed responses from agent and translate to Slack API

```go
func (a *SlackAdapter) StreamResponse(ctx context.Context, req *StreamResponseRequest) error {
    for {
        response, err := req.Stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("stream error: %w", err)
        }

        // Route based on payload type
        switch payload := response.Payload.(type) {
        case *pb.AgentResponse_Status:
            if a.aiFeatures {
                a.handleStatusUpdate(ctx, response.ConversationId, payload.Status)
            }

        case *pb.AgentResponse_Content:
            a.handleContentChunk(ctx, response.ConversationId, payload.Content)

        case *pb.AgentResponse_Prompts:
            if a.aiFeatures {
                a.handleSuggestedPrompts(ctx, response.ConversationId, payload.Prompts)
            }

        case *pb.AgentResponse_ThreadMetadata:
            a.handleThreadMetadata(ctx, payload.ThreadMetadata)

        case *pb.AgentResponse_Error:
            a.handleError(ctx, response.ConversationId, payload.Error)
        }
    }

    return nil
}

func (a *SlackAdapter) handleContentChunk(ctx context.Context, conversationID string, chunk *pb.ContentChunk) error {
    // Rate limit (3 second minimum)
    if err := a.rateLimiter.Wait(ctx); err != nil {
        return err
    }

    // Get conversation metadata from cache
    conv, ok := a.conversationCache.Get(conversationID)
    if !ok {
        return fmt.Errorf("conversation not found: %s", conversationID)
    }

    switch chunk.Type {
    case pb.ContentChunk_START:
        // Send initial message
        opts := []slack.MsgOption{
            slack.MsgOptionText(chunk.Content, false),
        }

        if conv.ThreadID != "" && a.autoThread {
            opts = append(opts, slack.MsgOptionTS(conv.ThreadID))
        }

        if chunk.Options != nil && chunk.Options.Ephemeral {
            opts = append(opts, slack.MsgOptionPostEphemeral(conv.UserID))
        }

        _, ts, err := a.api.PostMessageContext(ctx, conv.ChannelID, opts...)
        if err != nil {
            return fmt.Errorf("failed to post message: %w", err)
        }

        // Store message ID for potential updates
        conv.LastMessageID = ts

    case pb.ContentChunk_END:
        // Final message sent (no streaming, so just send complete message)
        return a.handleContentChunk(ctx, conversationID, &pb.ContentChunk{
            Type:    pb.ContentChunk_START,
            Content: chunk.Content,
            Options: chunk.Options,
        })

    case pb.ContentChunk_REPLACE:
        // Edit existing message (rare for Slack)
        if conv.LastMessageID != "" {
            _, _, _, err := a.api.UpdateMessageContext(ctx,
                conv.ChannelID,
                conv.LastMessageID,
                slack.MsgOptionText(chunk.Content, false),
            )
            return err
        }
    }

    return nil
}
```

### 7. AI Features Implementation

**Purpose**: Implement Slack AI-specific features

```go
// internal/adapter/slack/ai_features.go

func (a *SlackAdapter) SetStatus(ctx context.Context, conversationID string, status *pb.StatusUpdate) error {
    if !a.aiFeatures {
        return nil // Silently ignore if AI features disabled
    }

    // Get conversation from cache
    conv, ok := a.conversationCache.Get(conversationID)
    if !ok {
        return fmt.Errorf("conversation not found")
    }

    // Map proto status to Slack status message
    statusMessage := mapStatusToSlackMessage(status)

    // Call Slack AI API
    err := a.api.SetThreadStatus(ctx, slack.SetThreadStatusParams{
        ChannelID: conv.ChannelID,
        ThreadTS:  conv.ThreadID,
        Status:    statusMessage,
        Emoji:     status.Emoji,
    })

    if err != nil {
        log.Printf("[Slack] Failed to set status: %v", err)
        return err
    }

    return nil
}

func (a *SlackAdapter) SetSuggestedPrompts(ctx context.Context, conversationID string, prompts *pb.SuggestedPrompts) error {
    if !a.aiFeatures {
        return nil
    }

    conv, ok := a.conversationCache.Get(conversationID)
    if !ok {
        return fmt.Errorf("conversation not found")
    }

    // Convert proto prompts to Slack format
    slackPrompts := make([]slack.SuggestedPrompt, 0, len(prompts.Prompts))
    for _, p := range prompts.Prompts {
        slackPrompts = append(slackPrompts, slack.SuggestedPrompt{
            Title:   p.Title,
            Message: p.Message,
        })
    }

    // Call Slack AI API
    err := a.api.SetSuggestedPrompts(ctx, slack.SetSuggestedPromptsParams{
        ChannelID: conv.ChannelID,
        ThreadTS:  conv.ThreadID,
        Prompts:   slackPrompts,
    })

    if err != nil {
        log.Printf("[Slack] Failed to set suggested prompts: %v", err)
        return err
    }

    return nil
}

func mapStatusToSlackMessage(status *pb.StatusUpdate) string {
    switch status.Status {
    case pb.StatusUpdate_THINKING:
        return "Thinking..."
    case pb.StatusUpdate_SEARCHING:
        return "Searching knowledge base..."
    case pb.StatusUpdate_GENERATING:
        return "Generating response..."
    case pb.StatusUpdate_PROCESSING:
        return "Processing your request..."
    case pb.StatusUpdate_ANALYZING:
        return "Analyzing data..."
    case pb.StatusUpdate_CUSTOM:
        if status.CustomMessage != nil {
            return *status.CustomMessage
        }
        return "Processing..."
    default:
        return "Processing..."
    }
}
```

### 8. Thread History Storage & Hydration

**Purpose**: Store ground truth of Slack thread state to handle edits, deletes, and provide accurate context

**Why Needed**: Users can edit or delete messages in Slack. Agent needs accurate thread state.

```go
// internal/adapter/slack/thread_history.go

type ThreadHistoryStore struct {
    threads map[string]*ThreadHistory
    mu      sync.RWMutex
    maxSize int
    ttl     time.Duration
}

func NewThreadHistoryStore(maxSize int, ttl time.Duration) *ThreadHistoryStore {
    return &ThreadHistoryStore{
        threads: make(map[string]*ThreadHistory),
        maxSize: maxSize,
        ttl:     ttl,
    }
}

// Store incoming message in thread history
func (s *ThreadHistoryStore) AddMessage(conversationID string, msg *pb.ThreadMessage) {
    s.mu.Lock()
    defer s.mu.Unlock()

    thread, exists := s.threads[conversationID]
    if !exists {
        thread = &ThreadHistory{
            ConversationID: conversationID,
            Messages:       make([]*pb.ThreadMessage, 0, 50),
            LastFetched:    time.Now(),
            MaxMessages:    50,
        }
        s.threads[conversationID] = thread
    }

    thread.Messages = append(thread.Messages, msg)

    // Keep only last N messages
    if len(thread.Messages) > thread.MaxMessages {
        thread.Messages = thread.Messages[len(thread.Messages)-thread.MaxMessages:]
    }

    thread.LastFetched = time.Now()
}

// Update message (handle edits)
func (s *ThreadHistoryStore) UpdateMessage(conversationID string, messageID string, newContent string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    thread, exists := s.threads[conversationID]
    if !exists {
        return
    }

    for _, msg := range thread.Messages {
        if msg.MessageId == messageID {
            if !msg.WasEdited {
                // Store original content
                msg.OriginalContent = proto.String(msg.Content)
                msg.WasEdited = true
            }
            msg.Content = newContent
            msg.EditedAt = timestamppb.Now()
            break
        }
    }
}

// Mark message as deleted
func (s *ThreadHistoryStore) DeleteMessage(conversationID string, messageID string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    thread, exists := s.threads[conversationID]
    if !exists {
        return
    }

    for _, msg := range thread.Messages {
        if msg.MessageId == messageID {
            msg.IsDeleted = true
            msg.DeletedAt = timestamppb.Now()
            break
        }
    }
}

// Get thread history (for hydration)
func (s *ThreadHistoryStore) GetHistory(conversationID string, maxMessages int) *pb.ThreadHistoryResponse {
    s.mu.RLock()
    defer s.mu.RUnlock()

    thread, exists := s.threads[conversationID]
    if !exists {
        return &pb.ThreadHistoryResponse{
            ConversationId: conversationID,
            Messages:       []*pb.ThreadMessage{},
            IsComplete:     true,
            FetchedAt:      timestamppb.Now(),
        }
    }

    // Get last N messages
    messages := thread.Messages
    if len(messages) > maxMessages {
        messages = messages[len(messages)-maxMessages:]
    }

    return &pb.ThreadHistoryResponse{
        ConversationId: conversationID,
        Messages:       messages,
        IsComplete:     len(messages) <= maxMessages,
        FetchedAt:      timestamppb.New(thread.LastFetched),
    }
}
```

### 9. Handling Message Edits and Deletes

**Purpose**: Capture Slack edit/delete events and update thread history

```go
func (a *SlackAdapter) handleMessageChanged(ctx context.Context, ev *slackevents.MessageEvent) {
    // MessageChanged subtype indicates edit
    if ev.SubType != "message_changed" {
        return
    }

    conversationID := ev.Channel
    if ev.ThreadTimeStamp != "" {
        conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
    }

    // Extract message details
    message := ev.Message
    if message == nil {
        return
    }

    log.Printf("[Slack] Message edited: %s in %s", message.TimeStamp, conversationID)

    // Update thread history
    a.threadHistoryStore.UpdateMessage(
        conversationID,
        message.TimeStamp,
        message.Text,
    )

    // Optionally notify agent of edit via gRPC stream
    if a.agentStream != nil {
        feedback := &pb.PlatformFeedback{
            ConversationId: conversationID,
            Timestamp:      timestamppb.Now(),
            Feedback: &pb.PlatformFeedback_MessageEdit{
                MessageEdit: &pb.MessageEdit{
                    MessageId:  message.TimeStamp,
                    NewContent: message.Text,
                },
            },
        }

        // Send over bidirectional stream
        a.agentStream.Send(&pb.ConversationRequest{
            Request: &pb.ConversationRequest_Feedback{
                Feedback: feedback,
            },
        })
    }
}

func (a *SlackAdapter) handleMessageDeleted(ctx context.Context, ev *slackevents.MessageEvent) {
    if ev.SubType != "message_deleted" {
        return
    }

    conversationID := ev.Channel
    if ev.ThreadTimeStamp != "" {
        conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
    }

    deletedTS := ev.DeletedTimeStamp
    if deletedTS == "" {
        return
    }

    log.Printf("[Slack] Message deleted: %s in %s", deletedTS, conversationID)

    // Mark as deleted in thread history
    a.threadHistoryStore.DeleteMessage(conversationID, deletedTS)

    // Notify agent
    if a.agentStream != nil {
        feedback := &pb.PlatformFeedback{
            ConversationId: conversationID,
            Timestamp:      timestamppb.Now(),
            Feedback: &pb.PlatformFeedback_MessageDelete{
                MessageDelete: &pb.MessageDelete{
                    MessageId: deletedTS,
                },
            },
        }

        a.agentStream.Send(&pb.ConversationRequest{
            Request: &pb.ConversationRequest_Feedback{
                Feedback: feedback,
            },
        })
    }
}
```

### 10. Thread Hydration from Slack API

**Purpose**: Fetch thread history from Slack when cache miss or refresh needed

```go
func (a *SlackAdapter) GetThreadHistory(ctx context.Context, req *pb.ThreadHistoryRequest) (*pb.ThreadHistoryResponse, error) {
    // Check if in cache
    cached := a.threadHistoryStore.GetHistory(req.ConversationId, int(req.MaxMessages))

    // If cache is fresh (< 5 minutes old), return it
    if time.Since(cached.FetchedAt.AsTime()) < 5*time.Minute && cached.IsComplete {
        log.Printf("[Slack] Returning cached thread history for %s", req.ConversationId)
        return cached, nil
    }

    // Cache miss or stale - fetch from Slack API
    log.Printf("[Slack] Hydrating thread from Slack API: %s", req.ConversationId)

    if err := a.hydrateThreadFromSlack(ctx, req.ConversationId); err != nil {
        return nil, fmt.Errorf("failed to hydrate thread: %w", err)
    }

    // Return fresh data
    return a.threadHistoryStore.GetHistory(req.ConversationId, int(req.MaxMessages)), nil
}

func (a *SlackAdapter) hydrateThreadFromSlack(ctx context.Context, conversationID string) error {
    // Parse conversation ID to get channel and thread
    parts := strings.Split(conversationID, "-")
    if len(parts) < 1 {
        return fmt.Errorf("invalid conversation ID format")
    }

    channelID := parts[0]
    var threadTS string
    if len(parts) == 2 {
        threadTS = parts[1]
    }

    // Fetch messages from Slack
    var messages []slack.Message
    var err error

    if threadTS != "" {
        // Fetch thread replies
        msgs, _, _, err := a.api.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
            ChannelID: channelID,
            Timestamp: threadTS,
            Limit:     50,
        })
        if err != nil {
            return fmt.Errorf("failed to fetch thread: %w", err)
        }
        messages = msgs
    } else {
        // Fetch channel history
        history, err := a.api.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
            ChannelID: channelID,
            Limit:     50,
        })
        if err != nil {
            return fmt.Errorf("failed to fetch history: %w", err)
        }
        messages = history.Messages
    }

    // Store in thread history
    for _, msg := range messages {
        if msg.Type != "message" || msg.SubType == "bot_message" {
            continue
        }

        threadMsg := &pb.ThreadMessage{
            MessageId: msg.Timestamp,
            User: &pb.User{
                Id:       msg.User,
                Username: msg.Username,
            },
            Content:     msg.Text,
            Timestamp:   timestamppb.New(parseSlackTimestamp(msg.Timestamp)),
            WasEdited:   msg.Edited != nil,
            PlatformData: map[string]string{
                "team":    msg.Team,
                "subtype": msg.SubType,
            },
        }

        if msg.Edited != nil {
            threadMsg.EditedAt = timestamppb.New(time.Unix(int64(msg.Edited.Timestamp), 0))
        }

        a.threadHistoryStore.AddMessage(conversationID, threadMsg)
    }

    log.Printf("[Slack] Hydrated %d messages for %s", len(messages), conversationID)
    return nil
}
```

### 11. Adaptive Rate Limiting

**Purpose**: Debounce updates to respect Slack's 3-second minimum between updates

**Key Insight**: Slack's 3-second rate limit means we should batch/debounce updates rather than sending every incremental change. This provides better UX than throttling.

```go
// internal/adapter/slack/rate_limiter.go

type AdaptiveRateLimiter struct {
    minInterval  time.Duration // 3 seconds for Slack
    lastUpdate   time.Time
    pendingTimer *time.Timer
    mu           sync.Mutex
}

func NewAdaptiveRateLimiter(hz float64) *AdaptiveRateLimiter {
    return &AdaptiveRateLimiter{
        minInterval: time.Duration(1.0 / hz * float64(time.Second)),
        lastUpdate:  time.Time{},
    }
}

func (rl *AdaptiveRateLimiter) Wait(ctx context.Context) error {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(rl.lastUpdate)

    // If enough time has passed, allow immediate update
    if elapsed >= rl.minInterval {
        rl.lastUpdate = now
        return nil
    }

    // Otherwise, wait for remaining time
    waitTime := rl.minInterval - elapsed

    rl.mu.Unlock()
    select {
    case <-time.After(waitTime):
        rl.mu.Lock()
        rl.lastUpdate = time.Now()
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

// Debounce status updates (accumulate and send after delay)
type StatusDebouncer struct {
    limiter       *AdaptiveRateLimiter
    currentStatus *pb.StatusUpdate
    timer         *time.Timer
    callback      func(*pb.StatusUpdate)
    mu            sync.Mutex
}

func (d *StatusDebouncer) Update(status *pb.StatusUpdate) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.currentStatus = status

    // Reset timer (debounce)
    if d.timer != nil {
        d.timer.Stop()
    }

    d.timer = time.AfterFunc(500*time.Millisecond, func() {
        d.mu.Lock()
        status := d.currentStatus
        d.mu.Unlock()

        if status != nil {
            // Wait for rate limit, then send
            ctx := context.Background()
            if err := d.limiter.Wait(ctx); err == nil {
                d.callback(status)
            }
        }
    })
}
```

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

# AI Features (requires paid Slack plan)
SLACK_AI_FEATURES_ENABLED=true

# gRPC Connection
GRPC_AGENT_ADDRESS=localhost:9090
GRPC_MAX_MESSAGE_SIZE=4194304  # 4MB

# Conversation Cache (routing only)
CACHE_MAX_SIZE=1000
CACHE_TTL_MINUTES=60

# Thread History Storage (platform messages)
THREAD_HISTORY_MAX_SIZE=1000          # Max threads to store
THREAD_HISTORY_TTL_HOURS=24           # How long to keep
THREAD_HISTORY_MAX_MESSAGES=50        # Messages per thread
THREAD_HISTORY_REFRESH_MINUTES=5      # When to re-fetch from Slack
```

### Config Struct

```go
// config/config.go

type SlackConfig struct {
    // Core settings
    Enabled    bool   `env:"SLACK_ENABLED" default:"false"`
    BotToken   string `env:"SLACK_BOT_TOKEN"`
    AppToken   string `env:"SLACK_APP_TOKEN"`
    SocketMode bool   `env:"SLACK_SOCKET_MODE" default:"true"`
    AutoThread bool   `env:"SLACK_AUTO_THREAD" default:"true"`

    // AI Features
    AIFeaturesEnabled bool `env:"SLACK_AI_FEATURES_ENABLED" default:"false"`

    // gRPC
    GRPCAddress       string `env:"GRPC_AGENT_ADDRESS" default:"localhost:9090"`
    GRPCMaxMessageMB  int    `env:"GRPC_MAX_MESSAGE_SIZE" default:"4"`

    // Cache
    CacheMaxSize   int           `env:"CACHE_MAX_SIZE" default:"1000"`
    CacheTTL       time.Duration `env:"CACHE_TTL_MINUTES" default:"60m"`
}
```

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

   **For AI Features** (requires paid plan):
   - `assistant:write` - Use AI assistant features
   - `assistant:read` - Read assistant events

#### 4. Subscribe to Bot Events

1. Navigate to "Event Subscriptions"
2. Scroll to "Subscribe to bot events"
3. Add the following events:
   - `message.im` - Direct messages
   - `app_mention` - @mentions of the bot
   - `message_changed` - Message edits (CRITICAL for accuracy)
   - `message_deleted` - Message deletions
   - `reaction_added` - User reactions (feedback)
   - `reaction_removed` - Reaction removed

   **For AI Features**:
   - `assistant_thread_started` - AI assistant opened
   - `assistant_thread_context_changed` - Context changed

4. Click "Save Changes"

**Important**: `message_changed` and `message_deleted` events are essential for maintaining accurate thread history!

#### 5. Enable AI Assistant (Optional)

**Required for AI features**:
1. Navigate to "App Settings" → "AI & Automation"
2. Toggle "Enable AI Assistant" to ON
3. Configure assistant display name
4. Set assistant description (shown to users)
5. **Note**: Requires paid Slack workspace

#### 6. Install App to Workspace

1. Navigate to "OAuth & Permissions"
2. Click "Install to Workspace"
3. Review permissions and click "Allow"
4. Copy the "Bot User OAuth Token" (starts with `xoxb-`)

#### 7. Test Connection

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
    # Check if we need fresh thread history
    should_hydrate = (
        self.is_first_message(message.conversation_id) or
        self.received_edit_notification(message.conversation_id) or
        self.time_since_last_message(message.conversation_id) > 300  # 5 min
    )

    if should_hydrate:
        # Get accurate thread state from adapter
        thread_history = await self.get_thread_history(message.conversation_id)

        # Use thread history as context for AI
        context = self.build_context_from_thread(thread_history)
    else:
        # Use cached context
        context = self.get_cached_context(message.conversation_id)

    # Process with AI
    response = await self.llm.generate(context, message.content)

    # Send response
    await self.send_response(response)
```

## Message Flow

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
TranslateAppMentionEvent() → proto.Message
    ↓
Send Message over gRPC stream → ProcessConversation(stream)
    ↓
Agent receives proto.Message
    ↓
Agent: Send StatusUpdate(SEARCHING) via stream
    ↓
Adapter: assistant.threads.setStatus("Searching...")
    ↓
User sees "Searching..." in Slack
    ↓
Agent: Continue processing with AI/LLM
    ↓
Agent: Send StatusUpdate(GENERATING)
    ↓
Adapter: assistant.threads.setStatus("Generating response...")
    ↓
Agent: Send ContentChunk(START, "The weather is...")
    ↓
Adapter: chat.postMessage in thread
    ↓
User sees complete response in thread
```

### Message Edit Flow (Critical for Accuracy)

```
User sends: "What's the weather?"
    ↓
Adapter: Store in thread history
    ↓
Agent: Processes "What's the weather?"
    ↓
Agent: Responds "It's sunny today!"
    ↓
[User realizes they meant a specific city]
    ↓
User EDITS message to: "What's the weather in Paris?"
    ↓
Slack Platform → message_changed event
    ↓
Adapter: handleMessageChanged()
    ↓
Adapter: Update thread history (mark edited, update content)
    ↓
Adapter: Send PlatformFeedback(MessageEdit) to agent via stream
    ↓
Agent: Receives edit notification
    ↓
User sends: "And tomorrow?"
    ↓
Agent: Needs context, calls GetThreadHistory()
    ↓
Adapter: Returns thread with EDITED message
    ↓
Agent: Sees "What's the weather in Paris?" (edited)
    ↓
Agent: Responds correctly about Paris weather
    ↓
✅ Accurate response based on current thread state!
```

**Without thread history storage**:
- Agent would still see "What's the weather?" (original)
- Agent responds about general weather, not Paris ❌

**With thread history storage**:
- Adapter tracks the edit
- Agent gets accurate thread state
- Agent responds correctly ✅

### AI Assistant Thread Started Flow

```
User in Slack → Opens AI assistant container
    ↓
Slack Platform → assistant_thread_started event
    ↓
Socket Mode → Event to adapter
    ↓
handleAssistantThreadStarted()
    ↓
TranslateAssistantEvent() → proto.Message (empty content)
    ↓
Send over gRPC stream → Agent
    ↓
Agent: Detects new thread
    ↓
Agent: Send SuggestedPrompts with quick replies
    ↓
Adapter: assistant.threads.setSuggestedPrompts()
    ↓
User sees suggested prompts:
  "Summarize document"
  "Explain concept"
  "Answer question"
    ↓
User clicks a prompt
    ↓
Regular message flow continues
```

### Reaction Feedback Flow

```
Agent sends response in Slack
    ↓
User reacts with 👍 or 👎
    ↓
Slack Platform → reaction_added event
    ↓
Socket Mode → Event to adapter
    ↓
handleReactionAdded()
    ↓
TranslateFeedback() → proto.PlatformFeedback
    ↓
Send over gRPC stream → Agent
    ↓
Agent: Log feedback, update model performance
    ↓
No response to user (silent feedback)
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
Status: "Generating response..."  (after 3s, if still processing)
Message: "The weather is sunny" (complete, when done)
User Experience: Smooth, clear progress
```

---

## Error Handling

### Connection Errors

```go
case socketmode.EventTypeConnectionError:
    log.Printf("[Slack] Connection error: %v", evt.Data)
    a.setConnected(false)
    // Socket client will auto-reconnect
```

### Message Send Errors

```go
func (a *SlackAdapter) sendErrorMessage(ctx context.Context, channelID, threadID string, err error) {
    errorMsg := "Sorry, I encountered an error processing your message."

    a.api.PostMessageContext(ctx,
        channelID,
        slack.MsgOptionText(errorMsg, false),
        slack.MsgOptionTS(threadID),
    )
}
```

### Rate Limit Errors

Rate limiting is proactive (token bucket), but if Slack returns 429:

```go
if slackErr.StatusCode == 429 {
    retryAfter := slackErr.RetryAfter
    time.Sleep(retryAfter)
    // Retry request
}
```

---

## Testing

### Unit Tests

```go
// internal/adapter/slack/translator_test.go

func TestTranslateMessageEvent(t *testing.T) {
    event := &slackevents.MessageEvent{
        Type:      "message",
        User:      "U123ABC",
        Text:      "Hello bot",
        Channel:   "C456DEF",
        TimeStamp: "1234567890.123456",
    }

    msg := TranslateMessageEvent(event)

    assert.Equal(t, "slack", msg.Platform)
    assert.Equal(t, "Hello bot", msg.Content)
    assert.Equal(t, "U123ABC", msg.User.Id)
    assert.Equal(t, "C456DEF", msg.PlatformContext.ChannelId)
}

func TestStripMentions(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"<@U123ABC> hello", "hello"},
        {"hey <@U123ABC> how are you", "hey how are you"},
        {"no mentions here", "no mentions here"},
    }

    for _, tt := range tests {
        result := stripMentions(tt.input)
        assert.Equal(t, tt.expected, result)
    }
}

func TestMapStatusToSlackMessage(t *testing.T) {
    tests := []struct {
        status   pb.StatusUpdate_Status
        expected string
    }{
        {pb.StatusUpdate_THINKING, "Thinking..."},
        {pb.StatusUpdate_SEARCHING, "Searching knowledge base..."},
        {pb.StatusUpdate_GENERATING, "Generating response..."},
    }

    for _, tt := range tests {
        status := &pb.StatusUpdate{Status: tt.status}
        result := mapStatusToSlackMessage(status)
        assert.Equal(t, tt.expected, result)
    }
}
```

### Integration Tests

```go
// internal/adapter/slack/adapter_test.go

func TestThreadHistoryStore_EditMessage(t *testing.T) {
    store := NewThreadHistoryStore(100, time.Hour)

    // Add initial message
    msg := &pb.ThreadMessage{
        MessageId: "1234567890.123456",
        Content:   "What's the weather?",
        User:      &pb.User{Id: "U123", Username: "alice"},
        Timestamp: timestamppb.Now(),
    }

    store.AddMessage("C123-thread", msg)

    // Edit the message
    store.UpdateMessage("C123-thread", "1234567890.123456", "What's the weather in Paris?")

    // Get history
    history := store.GetHistory("C123-thread", 10)

    assert.Equal(t, 1, len(history.Messages))
    assert.True(t, history.Messages[0].WasEdited)
    assert.Equal(t, "What's the weather?", *history.Messages[0].OriginalContent)
    assert.Equal(t, "What's the weather in Paris?", history.Messages[0].Content)
}

func TestSlackAdapter_HydrateThreadFromSlack(t *testing.T) {
    adapter := NewSlackAdapter("xoxb-test", "xapp-test", SlackConfig{})

    // Mock Slack API client
    mockAPI := &MockSlackClient{
        replies: []slack.Message{
            {
                Type:      "message",
                User:      "U123",
                Text:      "Hello",
                Timestamp: "1234567890.123456",
            },
            {
                Type:      "message",
                User:      "U456",
                Text:      "Hi there",
                Timestamp: "1234567890.234567",
            },
        },
    }
    adapter.api = mockAPI

    // Initialize thread history store
    adapter.threadHistoryStore = NewThreadHistoryStore(100, time.Hour)

    // Hydrate from Slack
    err := adapter.hydrateThreadFromSlack(context.Background(), "C123-1234567890.123456")
    assert.NoError(t, err)

    // Check stored history
    history := adapter.threadHistoryStore.GetHistory("C123-1234567890.123456", 10)
    assert.Equal(t, 2, len(history.Messages))
    assert.Equal(t, "Hello", history.Messages[0].Content)
    assert.Equal(t, "Hi there", history.Messages[1].Content)
}

func TestSlackAdapter_StreamResponse(t *testing.T) {
    adapter := NewSlackAdapter("xoxb-test", "xapp-test", SlackConfig{
        AIFeaturesEnabled: true,
    })

    // Mock Slack client
    adapter.api = &MockSlackClient{}

    // Create mock gRPC stream
    mockStream := &MockAgentResponseStream{
        responses: []*pb.AgentResponse{
            {
                ConversationId: "C123-thread",
                Payload: &pb.AgentResponse_Status{
                    Status: &pb.StatusUpdate{Status: pb.StatusUpdate_THINKING},
                },
            },
            {
                ConversationId: "C123-thread",
                Payload: &pb.AgentResponse_Content{
                    Content: &pb.ContentChunk{
                        Type:    pb.ContentChunk_START,
                        Content: "Test response",
                    },
                },
            },
        },
    }

    // Setup conversation cache
    adapter.conversationCache.Put("C123-thread", &ConversationMetadata{
        ChannelID: "C123",
        UserID:    "U456",
    })

    err := adapter.StreamResponse(context.Background(), &StreamResponseRequest{
        Stream: mockStream,
    })

    assert.NoError(t, err)
}
```

### Manual Testing

1. Start messaging service with Slack adapter enabled
2. Start agent with gRPC server
3. Send `@BotName hello` in Slack channel
4. Verify:
   - Status update appears ("Thinking...")
   - Bot responds in thread
   - Response is complete (not streamed incrementally)

5. Test AI assistant (if enabled):
   - Open AI assistant container
   - Verify suggested prompts appear
   - Click a prompt, verify response

6. Test feedback:
   - React to bot message with 👍
   - Check agent logs for feedback event

7. Test rate limiting:
   - Send multiple status updates rapidly
   - Verify they're debounced (max 1 per 3 seconds)

---

## Production Considerations

### Graceful Shutdown

```go
func (a *SlackAdapter) Stop(ctx context.Context) error {
    log.Println("[Slack] Stopping adapter...")

    a.mu.Lock()
    a.connected = false
    a.mu.Unlock()

    // Close socket connection
    // Client handles cleanup automatically

    log.Println("[Slack] Adapter stopped")
    return nil
}
```

### Health Checks

```go
func (a *SlackAdapter) IsHealthy(ctx context.Context) bool {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.connected
}
```

### Monitoring Metrics

```go
var (
    slackMessagesReceived = prometheus.NewCounter(...)
    slackMessagesSent = prometheus.NewCounter(...)
    slackErrors = prometheus.NewCounterVec(...) // by error_type
    slackLatency = prometheus.NewHistogram(...)
)
```

### Logging

```go
// Connection events
log.Println("[Slack] Connected to Slack via Socket Mode")

// Message events
log.Printf("[Slack] Message: channel=%s, user=%s", channelID, userID)

// Errors
log.Printf("[Slack] Error: %v", err)
```

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
      - SLACK_AI_FEATURES_ENABLED=true
      - GRPC_LISTEN_ADDR=:9090

  agent:
    environment:
      - GRPC_AGENT_ADDRESS=messaging:9090
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
        - name: SLACK_AI_FEATURES_ENABLED
          value: "true"
        - name: GRPC_LISTEN_ADDR
          value: ":9090"
```

---

## Limitations and Future Enhancements

### Current Limitations

1. **No Content Streaming**: Intentional due to Slack's 3-second rate limit (poor UX)
2. **Text Only**: No Block Kit support yet (complex layouts)
3. **No Interactive Components**: Buttons, select menus not implemented
4. **No Slash Commands**: Only message-based interaction
5. **Single Workspace**: One bot per messaging instance
6. **AI Features Require Paid Plan**: Slack AI APIs not available on free workspaces

### Design Decisions (By Choice)

1. **No content streaming**: Status updates provide better UX than janky 3-second updates
2. **Thin adapter**: No business logic, pure protocol translation
3. **Agent owns context**: Adapter doesn't store conversation history
4. **gRPC over HTTP**: Better performance for real-time AI interactions

### Future Enhancements

#### Phase 2: Rich Messaging

- Block Kit support for rich layouts
- Interactive buttons and menus (translate to proto.CardAttachment)
- File attachments (upload/download)
- Image/video rendering

#### Phase 3: Advanced Features

- Slash commands → proto.Message with command metadata
- Shortcuts (message/global)
- Modal dialogs (views) for complex inputs
- Home tab customization

#### Phase 4: Enterprise

- Multi-workspace support (OAuth flow)
- User lookup and profile sync
- Workspace analytics
- Custom emoji support
- Thread title generation (automatic summaries)

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
- **gRPC connection failed** (check GRPC_AGENT_ADDRESS)
- Agent not listening on gRPC port

### gRPC Connection Issues

**Symptoms**: Adapter connects to Slack but messages don't reach agent

**Check**:
1. Agent gRPC server running: `netstat -an | grep 9090`
2. Network connectivity: `telnet messaging 9090`
3. gRPC logs: Look for "failed to connect" errors
4. Firewall rules blocking port 9090

**Fix**:
```bash
# Verify agent is listening
lsof -i :9090

# Test gRPC connection
grpcurl -plaintext localhost:9090 list

# Check Docker network
docker network inspect <network-name>
```

### AI Features Not Working

**Symptoms**: Status updates and suggested prompts not appearing

**Common issues**:
- `SLACK_AI_FEATURES_ENABLED=false` (check config)
- **Free Slack workspace** (requires paid plan)
- Bot not configured as AI assistant in Slack app settings
- Missing `assistant:write` scope

**Check**:
```bash
# Verify AI features enabled
echo $SLACK_AI_FEATURES_ENABLED

# Check Slack API response
# Look for "feature_not_available" errors in logs
```

### Connection Drops

Socket Mode auto-reconnects, but if persistent:

- Check network connectivity
- Verify tokens haven't been revoked
- Check Slack API status page
- **gRPC stream interrupted** (check agent health)

### Rate Limiting

If seeing "rate_limited" errors from Slack:

- **Status updates too frequent**: Check debouncing (should be max 1 per 3s)
- Message bursts: Adapter should handle automatically
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
- **Proto Style Guide**: https://protobuf.dev/programming-guides/style/

### Related Specs
- **gRPC AI Messaging Spec**: `/packages/astro-messaging/docs/grpc-ai-messaging-spec.md`
- **Discord Adapter Spec**: `/packages/astro-messaging/docs/discord-adapter-spec.md`
- **Teams Adapter Spec**: `/packages/astro-messaging/docs/teams-adapter-spec.md`

---

## Success Criteria

### Core Functionality
- ✅ Socket Mode connection established and maintained
- ✅ Receives messages in DMs and via @mentions
- ✅ gRPC bidirectional streaming with agent
- ✅ Responds with complete messages (no janky streaming)
- ✅ Threading works correctly (auto-thread in channels)
- ✅ Graceful reconnection on Slack disconnect
- ✅ Health checks report accurate status

### AI Features
- ✅ Status updates display correctly ("Thinking...", "Generating...")
- ✅ Suggested prompts appear when AI assistant opened
- ✅ 3-second rate limit respected (debouncing works)
- ✅ assistant_thread_started events processed
- ✅ Reaction feedback sent to agent

### Protocol Translation
- ✅ Slack events → proto.Message translation accurate
- ✅ proto.AgentResponse → Slack API calls work
- ✅ Attachments translated correctly
- ✅ User metadata populated properly

### Performance
- ✅ Adaptive rate limiting prevents 429 errors
- ✅ Conversation cache lookups fast (<1ms)
- ✅ gRPC stream handles backpressure
- ✅ Works alongside other adapters (Discord, Teams)

---

**End of Specification**
