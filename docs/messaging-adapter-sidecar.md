# Messaging Adapter Sidecar - Design Specification

## Overview

This document specifies a **Go-based sidecar container** that acts as a messaging adapter for Astro agents. The sidecar handles all platform-specific messaging operations (Slack, Discord, etc.) and communicates with the main agent container via a simple HTTP/gRPC API, following the sidecar pattern described in Astro's container integration.

## Architecture

### Sidecar Pattern

```
┌─────────────────────────────────────────────────────────────────┐
│                     Container Pod / Service                     │
│                                                                 │
│  ┌───────────────────────────┐  ┌──────────────────────────┐  │
│  │   Messaging Sidecar       │  │    Agent Container       │  │
│  │   (Go Binary)             │  │    (Any Language)        │  │
│  │                           │  │                          │  │
│  │  ┌─────────────────────┐  │  │  ┌────────────────────┐ │  │
│  │  │  Slack Adapter      │  │  │  │  AstroAgent        │ │  │
│  │  │  - Events API       │  │  │  │  - Mastra Core     │ │  │
│  │  │  - Socket Mode      │  │  │  │  - Tools/Graphs    │ │  │
│  │  │  - Rate Limiting    │  │  │  │  - Memory          │ │  │
│  │  └─────────────────────┘  │  │  └────────────────────┘ │  │
│  │                           │  │                          │  │
│  │  ┌─────────────────────┐  │  │                          │  │
│  │  │  Discord Adapter    │  │  │                          │  │
│  │  │  - Gateway          │  │  │                          │  │
│  │  │  - Intents          │  │  │                          │  │
│  │  │  - Rate Limiting    │  │  │                          │  │
│  │  └─────────────────────┘  │  │                          │  │
│  │                           │  │                          │  │
│  │  ┌─────────────────────┐  │  │                          │  │
│  │  │  HTTP/gRPC Server   │◄─┼──┼──►  HTTP Client         │  │
│  │  │  Port: 8081         │  │  │     to :8081            │  │
│  │  └─────────────────────┘  │  │                          │  │
│  │                           │  │                          │  │
│  │  localhost:8081          │  │     localhost:8080       │  │
│  └───────────────────────────┘  └──────────────────────────┘  │
│         ▲                                                      │
│         │ Platform Events                                     │
└─────────┼────────────────────────────────────────────────────┘
          │
     ┌────┴─────┐
     │  Slack   │
     │ Discord  │
     └──────────┘
```

### Communication Flow

1. **Inbound (Platform → Agent):**
   - Platform sends event to sidecar (webhook/websocket)
   - Sidecar translates to unified format
   - Sidecar calls agent container via HTTP POST `/message`
   - Agent processes and returns response
   - Sidecar sends response back to platform

2. **Outbound (Agent → Platform):**
   - Agent calls sidecar `/send-message` endpoint
   - Sidecar translates to platform-specific format
   - Sidecar sends to platform via API

3. **Streaming:**
   - Agent streams response chunks to sidecar
   - Sidecar accumulates and periodically updates platform message
   - Respects rate limits and platform constraints

## Architectural Decision: Single vs Multiple Sidecars

### The Question

Should we build **one sidecar for all platforms** or **separate sidecars per platform**?

### Option 1: Single Unified Sidecar (Recommended)

```
┌─────────────────────────────┐
│   Messaging Sidecar (Go)    │
│  ┌────────────────────────┐ │
│  │   Slack Adapter        │ │
│  │   Discord Adapter      │ │
│  │   Teams Adapter        │ │
│  └────────────────────────┘ │
│         Shared:             │
│  - Rate Limiters            │
│  - Conversation Store       │
│  - HTTP Server              │
└─────────────────────────────┘
```

**Advantages:**
- ✅ **Simpler deployment** - One container to manage
- ✅ **Resource efficient** - ~50-100MB total vs 100MB per platform
- ✅ **Shared conversation state** - Natural cross-platform context
- ✅ **Code reuse** - Common rate limiting, storage, translation
- ✅ **Single config point** - Enable/disable platforms with flags
- ✅ **Lower latency** - One localhost hop
- ✅ **Easier observability** - One set of metrics/logs
- ✅ **Cross-platform features** - User switches from Slack to Discord seamlessly

**Trade-offs:**
- ⚠️ **Coupled deployment** - Update Slack = redeploy everything
- ⚠️ **No independent scaling** - Can't scale Discord without Slack
- ⚠️ **Larger blast radius** - Bug in one adapter could affect all
- ⚠️ **Bigger binary** - All dependencies bundled (~30-50MB)

### Option 2: Separate Sidecars Per Platform

```
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│  Slack Sidecar   │  │ Discord Sidecar  │  │  Teams Sidecar   │
│  - Slack Only    │  │ - Discord Only   │  │  - Teams Only    │
└────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
         │                     │                      │
         └─────────────────────┼──────────────────────┘
                          ┌────▼────┐
                          │  Agent  │
                          └─────────┘
```

**Advantages:**
- ✅ **Independent scaling** - Scale Discord 10x without touching Slack
- ✅ **Isolated failures** - Discord crash doesn't affect Slack
- ✅ **Independent deployment** - Update one platform at a time
- ✅ **Smaller binaries** - Only platform-specific dependencies (~10-20MB each)
- ✅ **Platform optimization** - Tune each separately
- ✅ **Flexible routing** - Different agents per platform

**Trade-offs:**
- ⚠️ **More containers** - 2-3x the operational overhead
- ⚠️ **Duplicated code** - Repeated logic across sidecars
- ⚠️ **Higher resource usage** - 100MB+ per sidecar (300MB for 3 platforms)
- ⚠️ **Complex conversation state** - Need shared Redis/DB for cross-platform context
- ⚠️ **Multiple API endpoints** - Agent needs to know which sidecar to call
- ⚠️ **Harder cross-platform features** - User context split across containers

### Our Recommendation: Single Sidecar with Platform Toggle

We recommend **a single unified sidecar** for the following reasons:

#### 1. Most Use Cases Are Simple

```yaml
# Typical deployment: 1-2 platforms max
sidecars:
  - name: messaging
    environment:
      - SLACK_ENABLED=true
      - DISCORD_ENABLED=true
      - TEAMS_ENABLED=false
```

**Reality:** 95% of agents will use 1-2 platforms. The complexity of separate sidecars isn't justified for most deployments.

#### 2. Resource Efficiency Matters

```
Single Sidecar:  100MB memory, 0.2 CPU
Separate (x3):   300MB memory, 0.6 CPU
```

For 100 agent deployments:
- **Single:** 10GB memory total
- **Separate:** 30GB memory total

**That's real cost savings** in cloud environments.

#### 3. Cross-Platform Context is Valuable

Users might message on Slack, then continue on Discord. With a single sidecar:
- Shared conversation store
- Unified user context
- Seamless experience across platforms

#### 4. Operational Simplicity

```bash
# Single sidecar - one place to look
kubectl logs agent-pod -c messaging

# Separate sidecars - 3x the complexity
kubectl logs agent-pod -c messaging-slack
kubectl logs agent-pod -c messaging-discord
kubectl logs agent-pod -c messaging-teams
```

**3x the monitoring, alerting, and debugging surface area.**

#### 5. Design for Flexibility

The key is making the architecture **separable** even if deployed together:

```go
// Each adapter is completely independent
type Adapter interface {
    Initialize(ctx context.Context, config Config) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    // ...
}

// Can be compiled into separate binaries if needed
func main() {
    adapters := []adapter.Adapter{}

    if cfg.Slack.Enabled {
        adapters = append(adapters, slack.New())
    }
    if cfg.Discord.Enabled {
        adapters = append(adapters, discord.New())
    }

    // Start all enabled adapters
    for _, adapter := range adapters {
        go adapter.Start(ctx)
    }
}
```

### Hybrid Approach: Best of Both Worlds

Build **one codebase** with **multiple deployment modes**:

```go
// cmd/sidecar/main.go
func main() {
    mode := os.Getenv("DEPLOYMENT_MODE")

    switch mode {
    case "all":
        // Run all enabled platforms (default)
        startAllAdapters()
    case "slack":
        // Run only Slack
        startSlackAdapter()
    case "discord":
        // Run only Discord
        startDiscordAdapter()
    case "teams":
        // Run only Teams
        startTeamsAdapter()
    }
}
```

**Configuration examples:**

```yaml
# Development & most production: Single sidecar
DEPLOYMENT_MODE=all
SLACK_ENABLED=true
DISCORD_ENABLED=true

# High-scale production: Separate when needed
# Slack Pod:
DEPLOYMENT_MODE=slack
SLACK_ENABLED=true

# Discord Pod:
DEPLOYMENT_MODE=discord
DISCORD_ENABLED=true
```

This gives you:
- ✅ Simple default (one sidecar)
- ✅ Flexibility to separate later
- ✅ Same codebase (easy maintenance)
- ✅ Zero code duplication

### When to Use Separate Sidecars

Only in these specific scenarios:

#### 1. Massive Scale Differences
```
Slack:     100,000 users, 1M messages/day
Discord:   1,000 users, 10K messages/day
```
→ Scale Slack independently with more replicas

#### 2. Different SLAs
```
Slack:     Production, 99.9% uptime required
Discord:   Beta testing, 95% uptime acceptable
```
→ Isolate failures to protect production traffic

#### 3. Regulatory Requirements
```
Slack:     Enterprise customers, strict compliance audit trail
Discord:   Community users, relaxed rules
```
→ Separate deployment pipelines and data isolation

#### 4. Multi-Tenant Deployments
```
Customer A: Slack only, deployed in their VPC
Customer B: Discord only, deployed in their VPC
Customer C: Both platforms, shared infrastructure
```
→ Deploy only what each customer needs

### Migration Path

**Start Simple:**
```yaml
# Phase 1: Single sidecar (day 1)
sidecars:
  - name: messaging
    image: messaging:latest
    env:
      - DEPLOYMENT_MODE=all
```

**Scale When Needed:**
```yaml
# Phase 2: Separate only when you hit scale issues
sidecars:
  - name: messaging-slack
    image: messaging:latest
    env:
      - DEPLOYMENT_MODE=slack
    replicas: 5  # Scale Slack heavily

  - name: messaging-discord
    image: messaging:latest
    env:
      - DEPLOYMENT_MODE=discord
    replicas: 1  # Discord doesn't need scale
```

### Implementation Strategy

The codebase is designed to support both modes:

1. **Modular adapter design** - Each platform is completely isolated
2. **Configuration toggles** - Enable/disable via environment variables
3. **Deployment mode flag** - Support running all or specific platforms
4. **Shared infrastructure** - Rate limiting, storage, observability are reusable
5. **Zero cross-dependencies** - No adapter depends on another

**Result:** You get simplicity now with flexibility for the future. You can always split later if needed, but you won't need to for 95% of deployments.

**Start simple, scale smart.**

## Sidecar API Specification

The sidecar exposes a simple HTTP API that the agent container uses.

### API Endpoints

#### 1. Health Check

```
GET /health
```

**Response:**
```json
{
  "status": "healthy",
  "adapters": {
    "slack": "connected",
    "discord": "connected"
  },
  "uptime": 3600
}
```

#### 2. Receive Message (called by sidecar → agent)

The agent container must implement this endpoint for the sidecar to send incoming messages.

```
POST /message
```

**Request Body:**
```json
{
  "id": "msg-123",
  "platform": "slack",
  "platform_message_id": "1234567890.123456",
  "content": "Hello, how can you help me?",
  "user_id": "U123456",
  "user_name": "John Doe",
  "channel_id": "C123456",
  "thread_id": "1234567890.123456",
  "conversation_id": "C123456-1234567890.123456",
  "timestamp": "2026-01-15T10:30:00Z",
  "metadata": {
    "team_id": "T123456"
  }
}
```

**Response:**
```json
{
  "content": "I can help you with...",
  "attachments": [],
  "create_thread": false,
  "ephemeral": false
}
```

Or for streaming:
```json
{
  "stream": true,
  "stream_url": "http://localhost:8080/stream/session-123"
}
```

#### 3. Send Message (called by agent → sidecar)

```
POST /send-message
```

**Request Body:**
```json
{
  "platform": "slack",
  "channel_id": "C123456",
  "thread_id": "1234567890.123456",
  "content": "Here's the information you requested...",
  "attachments": [],
  "ephemeral": false,
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "message_id": "1234567890.654321",
  "timestamp": "2026-01-15T10:30:05Z"
}
```

#### 4. Update Message (for streaming)

```
PUT /message/:message_id
```

**Request Body:**
```json
{
  "platform": "slack",
  "content": "Updated message content...",
  "metadata": {}
}
```

#### 5. Get Conversation Context

```
GET /conversation/:conversation_id
```

**Response:**
```json
{
  "conversation_id": "C123456-1234567890.123456",
  "platform": "slack",
  "channel_id": "C123456",
  "thread_id": "1234567890.123456",
  "user_id": "U123456",
  "message_count": 5,
  "created_at": "2026-01-15T10:00:00Z",
  "last_message_at": "2026-01-15T10:30:00Z",
  "metadata": {}
}
```

#### 6. Platform-Specific Actions

```
POST /platform/:platform/action
```

**Example - React to message:**
```json
{
  "action": "react",
  "message_id": "1234567890.123456",
  "channel_id": "C123456",
  "emoji": "thumbsup"
}
```

## Protocol Options

### Option 1: Simple HTTP (Recommended for MVP)

**Pros:**
- Simple to implement
- Language agnostic
- Easy debugging with curl
- Standard HTTP tooling

**Cons:**
- Slightly higher overhead than gRPC
- No built-in streaming (use SSE or chunked encoding)

### Option 2: gRPC

**Pros:**
- Efficient binary protocol
- Built-in streaming
- Type safety with protobuf

**Cons:**
- More complex setup
- Requires protobuf definitions
- Less human-readable debugging

**For MVP, we recommend HTTP with JSON** for simplicity and debugging. Can upgrade to gRPC later if needed.

## Go Implementation

### Project Structure

```
astro-messaging/
├── cmd/
│   └── sidecar/
│       └── main.go                  # Entry point
│
├── internal/
│   ├── adapter/
│   │   ├── adapter.go               # Base adapter interface
│   │   ├── slack/
│   │   │   ├── adapter.go           # Slack adapter
│   │   │   ├── translator.go        # Message translation
│   │   │   ├── rate_limiter.go      # Slack rate limiting
│   │   │   └── client.go            # Slack API client
│   │   └── discord/
│   │       ├── adapter.go           # Discord adapter
│   │       ├── translator.go        # Message translation
│   │       ├── rate_limiter.go      # Discord rate limiting
│   │       └── client.go            # Discord API client
│   │
│   ├── api/
│   │   ├── server.go                # HTTP server
│   │   ├── handlers.go              # Request handlers
│   │   └── types.go                 # API types
│   │
│   ├── store/
│   │   ├── store.go                 # Storage interface
│   │   ├── redis.go                 # Redis implementation
│   │   └── memory.go                # In-memory implementation
│   │
│   ├── agent/
│   │   └── client.go                # Agent HTTP client
│   │
│   └── observability/
│       ├── metrics.go               # Prometheus metrics
│       ├── tracing.go               # OpenTelemetry
│       └── logging.go               # Structured logging
│
├── pkg/
│   └── types/
│       └── message.go               # Shared types
│
├── config/
│   ├── config.go                    # Configuration
│   └── config.yaml                  # Default config
│
├── deployments/
│   ├── Dockerfile                   # Multi-stage build
│   └── docker-compose.yaml          # Local dev
│
├── docs/
│   └── API.md                       # API documentation
│
├── go.mod
├── go.sum
└── README.md
```

### Core Types

```go
// pkg/types/message.go
package types

import "time"

// UnifiedMessage represents a platform-agnostic message
type UnifiedMessage struct {
	ID                string                 `json:"id"`
	PlatformMessageID string                 `json:"platform_message_id"`
	Platform          string                 `json:"platform"`
	Content           string                 `json:"content"`
	UserID            string                 `json:"user_id"`
	UserName          string                 `json:"user_name,omitempty"`
	ChannelID         string                 `json:"channel_id"`
	ThreadID          string                 `json:"thread_id,omitempty"`
	ConversationID    string                 `json:"conversation_id"`
	Timestamp         time.Time              `json:"timestamp"`
	Attachments       []Attachment           `json:"attachments,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// AgentResponse represents a response from the agent
type AgentResponse struct {
	Content      string       `json:"content"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	CreateThread bool         `json:"create_thread,omitempty"`
	Ephemeral    bool         `json:"ephemeral,omitempty"`
	Stream       bool         `json:"stream,omitempty"`
	StreamURL    string       `json:"stream_url,omitempty"`
}

// SendMessageRequest represents a request to send a message to a platform
type SendMessageRequest struct {
	Platform    string                 `json:"platform"`
	ChannelID   string                 `json:"channel_id"`
	ThreadID    string                 `json:"thread_id,omitempty"`
	Content     string                 `json:"content"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Ephemeral   bool                   `json:"ephemeral,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Attachment represents a file or media attachment
type Attachment struct {
	Type     string `json:"type"` // file, image, video, audio, link
	URL      string `json:"url"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}
```

### Adapter Interface

```go
// internal/adapter/adapter.go
package adapter

import (
	"context"
	"github.com/astro/messaging/pkg/types"
)

// Adapter is the interface that all platform adapters must implement
type Adapter interface {
	// Initialize sets up the adapter with configuration
	Initialize(ctx context.Context, config Config) error

	// Start begins listening for platform events
	Start(ctx context.Context) error

	// Stop gracefully shuts down the adapter
	Stop(ctx context.Context) error

	// OnMessage registers a handler for incoming messages
	OnMessage(handler MessageHandler)

	// SendMessage sends a message to the platform
	SendMessage(ctx context.Context, req *types.SendMessageRequest) (*SendMessageResult, error)

	// UpdateMessage updates an existing message (for streaming)
	UpdateMessage(ctx context.Context, messageID string, content string) error

	// GetPlatformName returns the platform identifier
	GetPlatformName() string

	// IsHealthy checks if the adapter is connected and healthy
	IsHealthy(ctx context.Context) bool
}

// MessageHandler is called when a message is received
type MessageHandler func(ctx context.Context, msg *types.UnifiedMessage) (*types.AgentResponse, error)

// Config holds adapter configuration
type Config struct {
	BotToken    string
	AppToken    string // For Slack Socket Mode
	SocketMode  bool
	WebhookURL  string
	AutoThread  bool
	RateLimit   RateLimitConfig
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	RequestsPerSecond float64
	BurstSize         int
}

// SendMessageResult contains the result of sending a message
type SendMessageResult struct {
	MessageID string
	Timestamp string
	Error     error
}
```

### Slack Adapter Implementation

```go
// internal/adapter/slack/adapter.go
package slack

import (
	"context"
	"fmt"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/pkg/types"
)

type SlackAdapter struct {
	client       *slack.Client
	socketClient *socketmode.Client
	config       adapter.Config
	handler      adapter.MessageHandler
	rateLimiter  *RateLimiter
}

func New() *SlackAdapter {
	return &SlackAdapter{}
}

func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error {
	a.config = config

	// Initialize Slack client
	a.client = slack.New(
		config.BotToken,
		slack.OptionAppLevelToken(config.AppToken),
	)

	// Initialize socket mode client if enabled
	if config.SocketMode {
		a.socketClient = socketmode.New(
			a.client,
			socketmode.OptionDebug(false),
		)
	}

	// Initialize rate limiter
	a.rateLimiter = NewRateLimiter(
		config.RateLimit.RequestsPerSecond,
		config.RateLimit.BurstSize,
	)

	return nil
}

func (a *SlackAdapter) Start(ctx context.Context) error {
	if a.config.SocketMode {
		return a.startSocketMode(ctx)
	}
	return a.startEventsAPI(ctx)
}

func (a *SlackAdapter) startSocketMode(ctx context.Context) error {
	go func() {
		for evt := range a.socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}

				// Acknowledge event
				a.socketClient.Ack(*evt.Request)

				// Handle inner event
				a.handleEvent(ctx, eventsAPIEvent.InnerEvent)

			case socketmode.EventTypeInteractive:
				// Handle interactive events (buttons, modals, etc.)
				a.socketClient.Ack(*evt.Request)
			}
		}
	}()

	return a.socketClient.Run()
}

func (a *SlackAdapter) handleEvent(ctx context.Context, innerEvent slackevents.EventsAPIInnerEvent) {
	switch ev := innerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		// Ignore bot messages
		if ev.BotID != "" {
			return
		}

		// Translate to unified message
		unifiedMsg := a.translateMessage(ev)

		// Call handler
		if a.handler != nil {
			resp, err := a.handler(ctx, unifiedMsg)
			if err != nil {
				fmt.Printf("Error handling message: %v\n", err)
				return
			}

			// Send response
			if resp != nil {
				a.sendResponse(ctx, unifiedMsg, resp)
			}
		}

	case *slackevents.AppMentionEvent:
		// Handle app mentions
		unifiedMsg := a.translateAppMention(ev)
		if a.handler != nil {
			resp, err := a.handler(ctx, unifiedMsg)
			if err != nil {
				fmt.Printf("Error handling mention: %v\n", err)
				return
			}
			if resp != nil {
				a.sendResponse(ctx, unifiedMsg, resp)
			}
		}
	}
}

func (a *SlackAdapter) translateMessage(ev *slackevents.MessageEvent) *types.UnifiedMessage {
	conversationID := ev.Channel
	if ev.ThreadTimeStamp != "" {
		conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
	}

	return &types.UnifiedMessage{
		ID:                generateID(),
		PlatformMessageID: ev.TimeStamp,
		Platform:          "slack",
		Content:           stripMentions(ev.Text),
		UserID:            ev.User,
		ChannelID:         ev.Channel,
		ThreadID:          ev.ThreadTimeStamp,
		ConversationID:    conversationID,
		Timestamp:         parseSlackTimestamp(ev.TimeStamp),
		Metadata: map[string]interface{}{
			"team_id": ev.Team,
		},
	}
}

func (a *SlackAdapter) SendMessage(ctx context.Context, req *types.SendMessageRequest) (*adapter.SendMessageResult, error) {
	// Wait for rate limiter
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Send message
	_, ts, err := a.client.PostMessageContext(
		ctx,
		req.ChannelID,
		slack.MsgOptionText(req.Content, false),
		slack.MsgOptionTS(req.ThreadID),
	)

	if err != nil {
		return &adapter.SendMessageResult{Error: err}, err
	}

	return &adapter.SendMessageResult{
		MessageID: ts,
		Timestamp: ts,
	}, nil
}

func (a *SlackAdapter) UpdateMessage(ctx context.Context, messageID string, content string) error {
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	// Parse messageID as "channelID:timestamp"
	parts := parseMessageID(messageID)

	_, _, _, err := a.client.UpdateMessageContext(
		ctx,
		parts.ChannelID,
		parts.Timestamp,
		slack.MsgOptionText(content, false),
	)

	return err
}

func (a *SlackAdapter) GetPlatformName() string {
	return "slack"
}

func (a *SlackAdapter) IsHealthy(ctx context.Context) bool {
	_, err := a.client.AuthTestContext(ctx)
	return err == nil
}

func (a *SlackAdapter) OnMessage(handler adapter.MessageHandler) {
	a.handler = handler
}

func (a *SlackAdapter) Stop(ctx context.Context) error {
	// Graceful shutdown
	return nil
}
```

### Discord Adapter Implementation

```go
// internal/adapter/discord/adapter.go
package discord

import (
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/pkg/types"
)

type DiscordAdapter struct {
	session     *discordgo.Session
	config      adapter.Config
	handler     adapter.MessageHandler
	rateLimiter *RateLimiter
}

func New() *DiscordAdapter {
	return &DiscordAdapter{}
}

func (a *DiscordAdapter) Initialize(ctx context.Context, config adapter.Config) error {
	a.config = config

	// Create Discord session
	session, err := discordgo.New("Bot " + config.BotToken)
	if err != nil {
		return fmt.Errorf("error creating Discord session: %w", err)
	}

	a.session = session

	// Set intents
	a.session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	// Initialize rate limiter
	a.rateLimiter = NewRateLimiter(
		config.RateLimit.RequestsPerSecond,
		config.RateLimit.BurstSize,
	)

	return nil
}

func (a *DiscordAdapter) Start(ctx context.Context) error {
	// Register event handlers
	a.session.AddHandler(a.handleMessageCreate)

	// Open connection
	if err := a.session.Open(); err != nil {
		return fmt.Errorf("error opening Discord connection: %w", err)
	}

	return nil
}

func (a *DiscordAdapter) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bot messages
	if m.Author.Bot {
		return
	}

	// Ignore messages that don't mention the bot (in guilds)
	if m.GuildID != "" {
		mentioned := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				mentioned = true
				break
			}
		}
		if !mentioned {
			return
		}
	}

	// Translate to unified message
	unifiedMsg := a.translateMessage(m)

	// Call handler
	if a.handler != nil {
		ctx := context.Background()
		resp, err := a.handler(ctx, unifiedMsg)
		if err != nil {
			fmt.Printf("Error handling message: %v\n", err)
			return
		}

		// Send response
		if resp != nil {
			a.sendResponse(ctx, unifiedMsg, resp)
		}
	}
}

func (a *DiscordAdapter) translateMessage(m *discordgo.MessageCreate) *types.UnifiedMessage {
	conversationID := m.ChannelID
	if m.Thread != nil {
		conversationID = fmt.Sprintf("%s-%s", m.ChannelID, m.Thread.ID)
	}

	return &types.UnifiedMessage{
		ID:                generateID(),
		PlatformMessageID: m.ID,
		Platform:          "discord",
		Content:           stripMentions(m.Content),
		UserID:            m.Author.ID,
		UserName:          m.Author.Username,
		ChannelID:         m.ChannelID,
		ThreadID:          getThreadID(m),
		ConversationID:    conversationID,
		Timestamp:         m.Timestamp,
		Metadata: map[string]interface{}{
			"guild_id": m.GuildID,
		},
	}
}

func (a *DiscordAdapter) SendMessage(ctx context.Context, req *types.SendMessageRequest) (*adapter.SendMessageResult, error) {
	// Wait for rate limiter
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Truncate if too long (Discord limit: 2000 chars)
	content := req.Content
	if len(content) > 2000 {
		content = content[:1997] + "..."
	}

	// Send message
	msg, err := a.session.ChannelMessageSend(req.ChannelID, content)
	if err != nil {
		return &adapter.SendMessageResult{Error: err}, err
	}

	return &adapter.SendMessageResult{
		MessageID: msg.ID,
		Timestamp: msg.Timestamp.String(),
	}, nil
}

func (a *DiscordAdapter) UpdateMessage(ctx context.Context, messageID string, content string) error {
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	// Parse messageID as "channelID:messageID"
	parts := parseMessageID(messageID)

	// Truncate if too long
	if len(content) > 2000 {
		content = content[:1997] + "..."
	}

	_, err := a.session.ChannelMessageEdit(parts.ChannelID, parts.MessageID, content)
	return err
}

func (a *DiscordAdapter) GetPlatformName() string {
	return "discord"
}

func (a *DiscordAdapter) IsHealthy(ctx context.Context) bool {
	return a.session != nil && a.session.DataReady
}

func (a *DiscordAdapter) OnMessage(handler adapter.MessageHandler) {
	a.handler = handler
}

func (a *DiscordAdapter) Stop(ctx context.Context) error {
	if a.session != nil {
		return a.session.Close()
	}
	return nil
}
```

### HTTP Server

```go
// internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/agent"
	"github.com/astro/messaging/pkg/types"
)

type Server struct {
	adapters    map[string]adapter.Adapter
	agentClient *agent.Client
	router      *mux.Router
	server      *http.Server
}

func NewServer(agentURL string, adapters map[string]adapter.Adapter) *Server {
	s := &Server{
		adapters:    adapters,
		agentClient: agent.NewClient(agentURL),
		router:      mux.NewRouter(),
	}

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Send message
	s.router.HandleFunc("/send-message", s.handleSendMessage).Methods("POST")

	// Update message
	s.router.HandleFunc("/message/{message_id}", s.handleUpdateMessage).Methods("PUT")

	// Get conversation
	s.router.HandleFunc("/conversation/{conversation_id}", s.handleGetConversation).Methods("GET")

	// Platform actions
	s.router.HandleFunc("/platform/{platform}/action", s.handlePlatformAction).Methods("POST")
}

func (s *Server) Start(ctx context.Context, addr string) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	// Register message handlers for all adapters
	for _, adapter := range s.adapters {
		adapter.OnMessage(s.handleIncomingMessage)
	}

	return s.server.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleIncomingMessage is called by adapters when a message arrives
func (s *Server) handleIncomingMessage(ctx context.Context, msg *types.UnifiedMessage) (*types.AgentResponse, error) {
	// Forward to agent container
	return s.agentClient.SendMessage(ctx, msg)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status": "healthy",
		"adapters": map[string]string{},
	}

	for name, adapter := range s.adapters {
		if adapter.IsHealthy(r.Context()) {
			status["adapters"].(map[string]string)[name] = "connected"
		} else {
			status["adapters"].(map[string]string)[name] = "disconnected"
		}
	}

	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req types.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	adapter, ok := s.adapters[req.Platform]
	if !ok {
		http.Error(w, "unknown platform", http.StatusBadRequest)
		return
	}

	result, err := adapter.SendMessage(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleUpdateMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	messageID := vars["message_id"]

	var req struct {
		Platform string `json:"platform"`
		Content  string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	adapter, ok := s.adapters[req.Platform]
	if !ok {
		http.Error(w, "unknown platform", http.StatusBadRequest)
		return
	}

	if err := adapter.UpdateMessage(r.Context(), messageID, req.Content); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
```

### Main Entry Point

```go
// cmd/sidecar/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/adapter/slack"
	"github.com/astro/messaging/internal/adapter/discord"
	"github.com/astro/messaging/internal/api"
	"github.com/astro/messaging/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Get deployment mode (all, slack, discord, teams)
	deploymentMode := os.Getenv("DEPLOYMENT_MODE")
	if deploymentMode == "" {
		deploymentMode = "all" // Default to running all enabled platforms
	}

	fmt.Printf("Starting messaging sidecar in mode: %s\n", deploymentMode)

	// Initialize adapters based on deployment mode
	adapters := initializeAdapters(ctx, cfg, deploymentMode)

	if len(adapters) == 0 {
		fmt.Printf("No adapters enabled or configured\n")
		os.Exit(1)
	}

	// Start all adapters
	for name, adapter := range adapters {
		go func(n string, a adapter.Adapter) {
			fmt.Printf("Starting %s adapter\n", n)
			if err := a.Start(ctx); err != nil {
				fmt.Printf("Error starting %s adapter: %v\n", n, err)
			}
		}(name, adapter)
	}

	// Start API server
	server := api.NewServer(cfg.AgentURL, adapters)
	go func() {
		fmt.Printf("Starting sidecar API server on %s\n", cfg.ListenAddr)
		if err := server.Start(ctx, cfg.ListenAddr); err != nil {
			fmt.Printf("Error starting server: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Shutting down gracefully...")

	// Stop all adapters
	for _, adapter := range adapters {
		adapter.Stop(ctx)
	}

	// Stop server
	server.Stop(ctx)
}

// initializeAdapters creates adapters based on deployment mode
func initializeAdapters(ctx context.Context, cfg *config.Config, mode string) map[string]adapter.Adapter {
	adapters := make(map[string]adapter.Adapter)

	// Determine which platforms to enable based on deployment mode
	enableSlack := false
	enableDiscord := false
	enableTeams := false

	switch mode {
	case "all":
		// Enable all platforms that are configured
		enableSlack = cfg.Slack.Enabled
		enableDiscord = cfg.Discord.Enabled
		enableTeams = cfg.Teams.Enabled
	case "slack":
		// Only Slack
		enableSlack = cfg.Slack.Enabled
	case "discord":
		// Only Discord
		enableDiscord = cfg.Discord.Enabled
	case "teams":
		// Only Teams
		enableTeams = cfg.Teams.Enabled
	default:
		fmt.Printf("Unknown deployment mode: %s, defaulting to 'all'\n", mode)
		enableSlack = cfg.Slack.Enabled
		enableDiscord = cfg.Discord.Enabled
		enableTeams = cfg.Teams.Enabled
	}

	// Initialize Slack adapter
	if enableSlack {
		slackAdapter := slack.New()
		if err := slackAdapter.Initialize(ctx, cfg.Slack.Config); err != nil {
			fmt.Printf("Error initializing Slack adapter: %v\n", err)
		} else {
			adapters["slack"] = slackAdapter
			fmt.Printf("Slack adapter initialized\n")
		}
	}

	// Initialize Discord adapter
	if enableDiscord {
		discordAdapter := discord.New()
		if err := discordAdapter.Initialize(ctx, cfg.Discord.Config); err != nil {
			fmt.Printf("Error initializing Discord adapter: %v\n", err)
		} else {
			adapters["discord"] = discordAdapter
			fmt.Printf("Discord adapter initialized\n")
		}
	}

	// Initialize Teams adapter (when implemented)
	if enableTeams {
		// teamsAdapter := teams.New()
		// if err := teamsAdapter.Initialize(ctx, cfg.Teams.Config); err != nil {
		// 	fmt.Printf("Error initializing Teams adapter: %v\n", err)
		// } else {
		// 	adapters["teams"] = teamsAdapter
		// 	fmt.Printf("Teams adapter initialized\n")
		// }
		fmt.Printf("Teams adapter not yet implemented\n")
	}

	return adapters
}
```

## Configuration

### Environment Variables

```bash
# Sidecar Configuration
LISTEN_ADDR=:8081
AGENT_URL=http://localhost:8080

# Deployment Mode
# Options: "all" (default), "slack", "discord", "teams"
# Use "all" to run all enabled platforms in one sidecar
# Use specific platform names to run only that platform (for separate scaling)
DEPLOYMENT_MODE=all

# Slack Configuration
SLACK_ENABLED=true
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token
SLACK_SOCKET_MODE=true
SLACK_AUTO_THREAD=true
SLACK_RATE_LIMIT_RPS=3

# Discord Configuration
DISCORD_ENABLED=true
DISCORD_BOT_TOKEN=your-discord-token
DISCORD_AUTO_THREAD=true
DISCORD_RATE_LIMIT_RPS=5

# Storage
REDIS_URL=redis://localhost:6379

# Observability
ENABLE_METRICS=true
METRICS_PORT=9090
LOG_LEVEL=info
```

### Configuration File

```yaml
# config/config.yaml
listen_addr: ":8081"
agent_url: "http://localhost:8080"

slack:
  enabled: true
  bot_token: ${SLACK_BOT_TOKEN}
  app_token: ${SLACK_APP_TOKEN}
  socket_mode: true
  auto_thread: true
  rate_limit:
    requests_per_second: 3
    burst_size: 10

discord:
  enabled: true
  bot_token: ${DISCORD_BOT_TOKEN}
  auto_thread: true
  rate_limit:
    requests_per_second: 5
    burst_size: 20

storage:
  type: redis
  redis_url: ${REDIS_URL}
  ttl: 604800  # 7 days

observability:
  metrics:
    enabled: true
    port: 9090
  logging:
    level: info
    format: json
```

## Astro Integration

### Astro Spec with Messaging Sidecar

```yaml
apiVersion: astro.dev/v1
kind: AgentInfrastructure

metadata:
  name: support-agent
  version: 1.0.0
  description: Customer support agent with Slack and Discord interfaces

# Main agent container
runtime:
  image: ghcr.io/myorg/support-agent:1.0.0
  port: 8080

  environment:
    - name: SIDECAR_URL
      value: http://localhost:8081

  healthCheck:
    type: http
    path: /health
    port: 8080

# Messaging sidecar
sidecars:
  - name: messaging
    image: ghcr.io/astro/messaging:latest
    port: 8081

    environment:
      - name: AGENT_URL
        value: http://localhost:8080
      - name: SLACK_ENABLED
        value: "true"
      - name: SLACK_BOT_TOKEN
        valueFrom:
          secretRef: slack-bot-token
      - name: SLACK_APP_TOKEN
        valueFrom:
          secretRef: slack-app-token
      - name: SLACK_SOCKET_MODE
        value: "true"
      - name: DISCORD_ENABLED
        value: "true"
      - name: DISCORD_BOT_TOKEN
        valueFrom:
          secretRef: discord-bot-token
      - name: REDIS_URL
        value: redis://redis:6379

    healthCheck:
      type: http
      path: /health
      port: 8081

    resources:
      cpu: "0.5"
      memory: "512Mi"

# Agent interface (exposed by main container)
interface:
  type: rest
  port: 8080
  endpoints:
    - path: /message
      method: POST
      description: Receive message from sidecar

# Agent configuration
models:
  primary:
    name: gpt-4-turbo
    provider: openai

tools:
  - name: knowledge_base
    type: function
    implementation: builtin

observability:
  tracing:
    enabled: true
    provider: langsmith
  metrics:
    enabled: true
    port: 9090
```

### Generated Docker Compose

```yaml
# Generated by astro dev
version: '3.8'

services:
  # Main agent container
  agent:
    image: ghcr.io/myorg/support-agent:1.0.0
    ports:
      - "8080:8080"
    environment:
      - SIDECAR_URL=http://messaging:8081
      - MODEL_ENDPOINT=${MODEL_ENDPOINT}
    depends_on:
      - messaging
      - redis
    networks:
      - agent-network

  # Messaging sidecar
  messaging:
    image: ghcr.io/astro/messaging:latest
    ports:
      - "8081:8081"
    environment:
      - AGENT_URL=http://agent:8080
      - SLACK_ENABLED=true
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
      - SLACK_SOCKET_MODE=true
      - DISCORD_ENABLED=true
      - DISCORD_BOT_TOKEN=${DISCORD_BOT_TOKEN}
      - REDIS_URL=redis://redis:6379
    depends_on:
      - redis
    networks:
      - agent-network

  # Redis for conversation state
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    networks:
      - agent-network

volumes:
  redis-data:

networks:
  agent-network:
```

## Dockerfile

```dockerfile
# Multi-stage build for Go sidecar
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sidecar ./cmd/sidecar

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/sidecar .

# Copy config
COPY config/config.yaml ./config/

EXPOSE 8081

CMD ["./sidecar"]
```

## Agent Container Implementation

The agent container needs to implement a simple HTTP endpoint to receive messages from the sidecar.

### TypeScript/Node.js Example

```typescript
// agent/src/index.ts
import express from 'express';
import { AstroAgent } from '@astro/agents';

const app = express();
app.use(express.json());

// Create agent
const agent = new AstroAgent()
  .meta({
    title: 'Support Agent',
    description: 'Customer support assistant',
  })
  .instructions('You are a helpful customer support assistant.');

// Endpoint called by sidecar
app.post('/message', async (req, res) => {
  const message = req.body;

  try {
    let response = '';

    // Stream agent response
    await agent.stream({
      prompt: message.content,
      threadId: message.conversation_id,
      userId: message.user_id,

      onChunk: (chunk) => {
        response += chunk;
      },

      onFinish: (result) => {
        response = result;
      },
    });

    // Return response to sidecar
    res.json({
      content: response,
      create_thread: true,
    });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

app.get('/health', (req, res) => {
  res.json({ status: 'healthy' });
});

const PORT = process.env.PORT || 8080;
app.listen(PORT, () => {
  console.log(`Agent listening on port ${PORT}`);
});
```

### Python Example

```python
# agent/main.py
from flask import Flask, request, jsonify
from mastra import Agent

app = Flask(__name__)

# Create agent
agent = Agent(
    name="Support Agent",
    description="Customer support assistant",
    instructions="You are a helpful customer support assistant."
)

@app.route('/message', methods=['POST'])
def handle_message():
    message = request.json

    try:
        # Process with agent
        response = agent.run(
            prompt=message['content'],
            thread_id=message['conversation_id'],
            user_id=message['user_id']
        )

        return jsonify({
            'content': response.text,
            'create_thread': True
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/health', methods=['GET'])
def health():
    return jsonify({'status': 'healthy'})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8080)
```

## Deployment

### Local Development

```bash
# 1. Start with docker-compose
docker-compose up

# 2. Or use Astro CLI
astro dev --watch

# 3. Test
curl http://localhost:8081/health
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: support-agent
spec:
  replicas: 2
  selector:
    matchLabels:
      app: support-agent
  template:
    metadata:
      labels:
        app: support-agent
    spec:
      containers:
        # Main agent container
        - name: agent
          image: ghcr.io/myorg/support-agent:1.0.0
          ports:
            - containerPort: 8080
          env:
            - name: SIDECAR_URL
              value: "http://localhost:8081"
          resources:
            requests:
              cpu: 500m
              memory: 1Gi
            limits:
              cpu: 1000m
              memory: 2Gi

        # Messaging sidecar
        - name: messaging
          image: ghcr.io/astro/messaging:latest
          ports:
            - containerPort: 8081
          env:
            - name: AGENT_URL
              value: "http://localhost:8080"
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
            - name: DISCORD_ENABLED
              value: "true"
            - name: DISCORD_BOT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: discord-credentials
                  key: bot-token
            - name: REDIS_URL
              value: "redis://redis-service:6379"
          resources:
            requests:
              cpu: 250m
              memory: 512Mi
            limits:
              cpu: 500m
              memory: 1Gi
```

## Testing

### Unit Tests

```go
// internal/adapter/slack/adapter_test.go
package slack

import (
	"context"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestSlackAdapter_TranslateMessage(t *testing.T) {
	adapter := New()

	slackMsg := &slackevents.MessageEvent{
		User:    "U123",
		Channel: "C123",
		Text:    "<@U456> Hello!",
		TimeStamp: "1234567890.123456",
	}

	unified := adapter.translateMessage(slackMsg)

	assert.Equal(t, "slack", unified.Platform)
	assert.Equal(t, "U123", unified.UserID)
	assert.Equal(t, "C123", unified.ChannelID)
	assert.NotContains(t, unified.Content, "<@U456>")
}
```

### Integration Tests

```go
// test/integration/sidecar_test.go
package integration

import (
	"testing"
	"net/http"
	"bytes"
)

func TestSidecarToAgent(t *testing.T) {
	// Start test sidecar
	sidecar := startTestSidecar(t)
	defer sidecar.Stop()

	// Start mock agent
	agent := startMockAgent(t)
	defer agent.Stop()

	// Send test message
	msg := map[string]interface{}{
		"platform": "slack",
		"content": "test message",
		"user_id": "U123",
		"channel_id": "C123",
	}

	resp, err := http.Post(
		sidecar.URL+"/send-message",
		"application/json",
		bytes.NewBuffer(marshal(msg)),
	)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
```

## Benefits of Sidecar Architecture

1. **Separation of Concerns**
   - Agent focuses on AI logic
   - Sidecar handles platform integration
   - Clear API boundary

2. **Language Independence**
   - Agent can be any language
   - Sidecar is Go (fast, low overhead)
   - Simple HTTP protocol

3. **Reusability**
   - Same sidecar works with any agent
   - Can be updated independently
   - Shared across multiple agents

4. **Resource Efficiency**
   - Single sidecar per agent pod
   - Shared network namespace
   - Low-latency localhost communication

5. **Operational Simplicity**
   - Deploy as single unit
   - Shared lifecycle
   - Kubernetes-native pattern

## Performance Characteristics

### Resource Usage

- **Memory**: ~50-100MB per sidecar
- **CPU**: <0.1 core idle, <0.5 core under load
- **Network**: Localhost only (no external latency)

### Latency

- Sidecar → Agent: <1ms (localhost)
- Agent → Sidecar: <1ms (localhost)
- Platform → Sidecar: Network dependent
- End-to-end: ~50-500ms depending on agent processing

## References

### Go Libraries

**Slack:**
- [slack-go/slack GitHub](https://github.com/slack-go/slack)
- [slack-go/slack Package Documentation](https://pkg.go.dev/github.com/slack-go/slack)
- [Slack API Tutorials](https://api.slack.com/tutorials/tags/go)

**Discord:**
- [bwmarrin/discordgo GitHub](https://github.com/bwmarrin/discordgo)
- [discordgo Package Documentation](https://pkg.go.dev/github.com/bwmarrin/discordgo)
- [Building Discord Bot in Go](https://medium.com/@mssandeepkamath/building-a-simple-discord-bot-using-go-12bfca31ad5d)

**Alternative Discord Libraries:**
- [Arikawa - Discord API Framework](https://github.com/diamondburned/arikawa)
- [Goscord - Discord Bot API](https://goscord.dev/)

### Architecture Patterns

- Astro Container Integration (container-integration.md)
- Sidecar pattern in Kubernetes/container orchestration

## Future Enhancements

### Additional Platforms

- Microsoft Teams adapter
- Telegram adapter
- WhatsApp Business API adapter
- Matrix protocol adapter

### Advanced Features

- Message queue integration (Kafka, RabbitMQ)
- Multi-agent routing
- A/B testing framework
- Advanced conversation analytics
- Human handoff protocol

### Performance Optimizations

- Connection pooling
- Message batching
- Redis cluster support
- Horizontal scaling with sharding
