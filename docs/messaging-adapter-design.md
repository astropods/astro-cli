# Messaging Adapter for Astro Agents - Design Specification

## Overview

This document specifies the design of a messaging adapter layer that enables Astro agents (built on Mastra) to interact with users through messaging platforms like Slack and Discord. The adapter provides a bidirectional bridge between platform-specific messaging APIs and the agent framework, handling message translation, context management, threading, and real-time communication.

## Design Philosophy

1. **Platform Abstraction**: Unified interface for all messaging platforms while preserving platform-specific capabilities
2. **Agent-Agnostic**: Works with any AstroAgent instance without requiring agent modifications
3. **Context Preservation**: Maintains conversation context across messages, threads, and sessions
4. **Real-Time Streaming**: Supports streaming agent responses for natural conversation flow
5. **Production Ready**: Built-in rate limiting, error handling, and observability
6. **Extensible**: Easy to add support for additional messaging platforms

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Messaging Platforms                         │
│  ┌──────────────┐              ┌──────────────┐                 │
│  │    Slack     │              │   Discord    │                 │
│  │  Events API  │              │   Gateway    │                 │
│  │  Webhooks    │              │  WebSocket   │                 │
│  └──────┬───────┘              └───────┬──────┘                 │
└─────────┼──────────────────────────────┼────────────────────────┘
          │                              │
          │  Platform Events             │  Platform Events
          ▼                              ▼
┌────────────────────────────────────────────────────────────────┐
│                      Adapter Layer                             │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │          Platform-Specific Adapters                      │  │
│  │  ┌─────────────────┐         ┌─────────────────┐         │  │
│  │  │  SlackAdapter   │         │ DiscordAdapter  │         │  │
│  │  │  - Events       │         │  - Gateway      │         │  │
│  │  │  - Socket Mode  │         │  - Intents      │         │  │
│  │  │  - Threading    │         │  - Threads      │         │  │
│  │  └────────┬────────┘         └────────┬────────┘         │  │
│  └───────────┼───────────────────────────┼──────────────────┘  │
│              │                           │                     │
│              └───────────┬───────────────┘                     │
│                          ▼                                     │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │           Unified Message Interface                      │  │
│  │  - Message Translation                                   │  │
│  │  - Context Management                                    │  │
│  │  - Session Tracking                                      │  │
│  └──────────────────────┬───────────────────────────────────┘  │
└─────────────────────────┼──────────────────────────────────────┘
                          │  Unified Message
                          ▼
┌────────────────────────────────────────────────────────────────┐
│                    Agent Interface                             │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              AgentMessageHandler                         │  │
│  │  - Route to AstroAgent                                   │  │
│  │  - Stream Management                                     │  │
│  │  - Response Formatting                                   │  │
│  └──────────────────────┬───────────────────────────────────┘  │
└─────────────────────────┼──────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     AstroAgent (Mastra)                         │
│  - Agent Logic                                                  │
│  - Tools/Workflows                                              │
│  - Memory/Context                                               │
│  - Streaming Responses                                          │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Unified Message Interface

Provides a platform-agnostic message format that all adapters translate to/from.

#### Message Types

```typescript
// Incoming message from user
interface UnifiedIncomingMessage {
  // Identifiers
  id: string;                          // Unique message ID
  platformMessageId: string;           // Platform-specific message ID

  // Content
  content: string;                     // Message text
  attachments?: MessageAttachment[];   // Files, images, etc.

  // Context
  userId: string;                      // User who sent the message
  userName?: string;                   // Display name
  userEmail?: string;                  // Email (if available)

  // Channel/Thread context
  channelId: string;                   // Channel/Server ID
  channelName?: string;                // Display name
  threadId?: string;                   // Thread ID (if in thread)
  threadTimestamp?: string;            // Thread parent timestamp (Slack)

  // Session management
  conversationId: string;              // Unique conversation identifier

  // Metadata
  platform: 'slack' | 'discord';
  timestamp: Date;
  metadata?: Record<string, unknown>;  // Platform-specific metadata
}

// Outgoing message to user
interface UnifiedOutgoingMessage {
  // Content
  content: string;                     // Message text
  attachments?: MessageAttachment[];   // Files, images, etc.

  // Formatting
  formatting?: MessageFormatting;      // Platform-specific formatting
  blocks?: any[];                      // Slack blocks or Discord embeds

  // Threading
  threadId?: string;                   // Reply in thread
  createThread?: boolean;              // Create new thread

  // Behavior
  ephemeral?: boolean;                 // Only visible to user
  updateMessageId?: string;            // Update existing message

  // Metadata
  metadata?: Record<string, unknown>;
}

interface MessageAttachment {
  type: 'file' | 'image' | 'video' | 'audio' | 'link';
  url: string;
  name?: string;
  mimeType?: string;
  size?: number;
}

interface MessageFormatting {
  markdown?: boolean;
  codeBlocks?: boolean;
  mentions?: boolean;
}
```

### 2. Base Adapter Interface

Abstract interface that all platform adapters implement.

```typescript
interface MessagingAdapter {
  // Lifecycle
  initialize(config: AdapterConfig): Promise<void>;
  start(): Promise<void>;
  stop(): Promise<void>;

  // Message handling
  onMessage(handler: MessageHandler): void;
  sendMessage(message: UnifiedOutgoingMessage, context: MessageContext): Promise<SendMessageResult>;

  // Streaming support
  sendStreamingMessage(
    stream: AsyncIterable<string>,
    context: MessageContext,
    options?: StreamingOptions
  ): Promise<SendMessageResult>;

  // Context management
  getConversationContext(conversationId: string): Promise<ConversationContext | null>;
  updateConversationContext(conversationId: string, context: ConversationContext): Promise<void>;

  // Platform-specific
  getPlatformName(): string;
  getPlatformCapabilities(): PlatformCapabilities;

  // Health
  isHealthy(): Promise<boolean>;
}

interface AdapterConfig {
  // Authentication
  botToken: string;
  appToken?: string;                   // For Slack Socket Mode

  // Connection
  socketMode?: boolean;                // Use websocket instead of webhooks
  webhookUrl?: string;                 // For webhook-based connections

  // Behavior
  autoCreateThreads?: boolean;         // Automatically thread responses
  preserveFormatting?: boolean;        // Keep markdown/code formatting
  ephemeralErrors?: boolean;           // Show errors only to user

  // Rate limiting
  rateLimit?: RateLimitConfig;

  // Logging
  logLevel?: 'debug' | 'info' | 'warn' | 'error';
  observability?: ObservabilityConfig;
}

type MessageHandler = (message: UnifiedIncomingMessage) => Promise<void>;

interface MessageContext {
  platform: string;
  channelId: string;
  threadId?: string;
  userId: string;
  conversationId: string;
}

interface SendMessageResult {
  success: boolean;
  messageId?: string;
  timestamp?: string;
  error?: string;
}

interface PlatformCapabilities {
  supportsThreads: boolean;
  supportsStreaming: boolean;
  supportsAttachments: boolean;
  supportsEphemeral: boolean;
  supportsMessageUpdates: boolean;
  supportsRichFormatting: boolean;
  maxMessageLength: number;
}
```

### 3. Slack Adapter

Implements the Slack-specific messaging logic.

#### Key Features

- **Events API**: Receives events via HTTP webhooks
- **Socket Mode**: Alternative WebSocket-based event delivery (no public URL required)
- **Threading**: Automatic thread management with `thread_ts`
- **Message Formatting**: Converts Slack's mrkdwn to unified format
- **Rate Limiting**: Respects Slack's tier-based rate limits
- **App Home**: Optional support for App Home tab interactions

#### Implementation Details

```typescript
class SlackAdapter implements MessagingAdapter {
  private client: WebClient;           // Slack Web API client
  private app: App;                    // Slack Bolt framework
  private conversationStore: ConversationStore;

  // Events to subscribe to
  private readonly REQUIRED_EVENTS = [
    'message.channels',                // Public channel messages
    'message.groups',                  // Private channel messages
    'message.im',                      // Direct messages
    'app_mention',                     // @mentions
  ];

  // Scopes required
  private readonly REQUIRED_SCOPES = [
    'chat:write',                      // Send messages
    'channels:history',                // Read channel history
    'groups:history',                  // Read private channel history
    'im:history',                      // Read DM history
    'users:read',                      // Get user info
    'app_mentions:read',               // Receive mentions
  ];

  async initialize(config: AdapterConfig): Promise<void> {
    // Initialize Slack Bolt app
    this.app = new App({
      token: config.botToken,
      socketMode: config.socketMode,
      appToken: config.appToken,
      signingSecret: config.signingSecret,
    });

    // Set up event listeners
    this.setupEventHandlers();

    // Initialize conversation store
    this.conversationStore = new ConversationStore();
  }

  private setupEventHandlers(): void {
    // Handle all message events
    this.app.message(async ({ message, say, client }) => {
      // Filter bot messages
      if (message.subtype === 'bot_message') return;

      // Translate to unified format
      const unifiedMessage = await this.translateIncomingMessage(message);

      // Emit to handler
      await this.messageHandler?.(unifiedMessage);
    });

    // Handle app mentions
    this.app.event('app_mention', async ({ event, say }) => {
      const unifiedMessage = await this.translateIncomingMessage(event);
      await this.messageHandler?.(unifiedMessage);
    });
  }

  private async translateIncomingMessage(
    slackMessage: any
  ): Promise<UnifiedIncomingMessage> {
    // Extract conversation ID (channel + thread)
    const conversationId = slackMessage.thread_ts
      ? `${slackMessage.channel}-${slackMessage.thread_ts}`
      : slackMessage.channel;

    return {
      id: generateId(),
      platformMessageId: slackMessage.ts,
      content: this.stripMentions(slackMessage.text),
      userId: slackMessage.user,
      channelId: slackMessage.channel,
      threadId: slackMessage.thread_ts,
      conversationId,
      platform: 'slack',
      timestamp: new Date(parseFloat(slackMessage.ts) * 1000),
      metadata: {
        teamId: slackMessage.team,
        eventType: slackMessage.type,
      },
    };
  }

  async sendMessage(
    message: UnifiedOutgoingMessage,
    context: MessageContext
  ): Promise<SendMessageResult> {
    try {
      const response = await this.client.chat.postMessage({
        channel: context.channelId,
        thread_ts: context.threadId,
        text: message.content,
        blocks: message.blocks,
      });

      return {
        success: true,
        messageId: response.ts as string,
        timestamp: response.ts as string,
      };
    } catch (error) {
      return {
        success: false,
        error: error.message,
      };
    }
  }

  async sendStreamingMessage(
    stream: AsyncIterable<string>,
    context: MessageContext,
    options?: StreamingOptions
  ): Promise<SendMessageResult> {
    // Post initial message
    const initialResponse = await this.sendMessage(
      { content: options?.initialMessage || '...' },
      context
    );

    if (!initialResponse.success) return initialResponse;

    // Stream updates by updating the message
    let accumulated = '';
    let lastUpdate = Date.now();
    const UPDATE_INTERVAL_MS = 1000; // Update every 1s to avoid rate limits

    for await (const chunk of stream) {
      accumulated += chunk;

      // Throttle updates
      if (Date.now() - lastUpdate >= UPDATE_INTERVAL_MS) {
        await this.client.chat.update({
          channel: context.channelId,
          ts: initialResponse.messageId!,
          text: accumulated,
        });
        lastUpdate = Date.now();
      }
    }

    // Final update
    await this.client.chat.update({
      channel: context.channelId,
      ts: initialResponse.messageId!,
      text: accumulated,
    });

    return initialResponse;
  }
}
```

#### Slack-Specific Considerations

1. **Rate Limits**: Slack has tiered rate limits. As of 2026, marketplace-approved apps have higher limits
2. **Message Updates**: Slack allows updating messages, enabling streaming UX
3. **Threading**: Use `thread_ts` to maintain conversation context
4. **Socket Mode vs Webhooks**: Socket Mode doesn't require public URL but needs `appToken`
5. **Scopes**: Ensure bot has required OAuth scopes during installation

### 4. Discord Adapter

Implements the Discord-specific messaging logic.

#### Key Features

- **Gateway WebSocket**: Maintains persistent connection for real-time events
- **Intents**: Subscribes to specific event types via Gateway Intents
- **Thread Support**: Native support for public/private threads
- **Rich Embeds**: Discord's structured message format
- **Slash Commands**: Optional support for slash command registration

#### Implementation Details

```typescript
class DiscordAdapter implements MessagingAdapter {
  private client: Client;              // Discord.js client
  private conversationStore: ConversationStore;

  // Required intents
  private readonly REQUIRED_INTENTS = [
    GatewayIntentBits.Guilds,          // Access to guild info
    GatewayIntentBits.GuildMessages,   // Receive guild messages
    GatewayIntentBits.MessageContent,  // Read message content
    GatewayIntentBits.DirectMessages,  // Receive DMs
  ];

  async initialize(config: AdapterConfig): Promise<void> {
    // Initialize Discord client
    this.client = new Client({
      intents: this.REQUIRED_INTENTS,
    });

    // Set up event handlers
    this.setupEventHandlers();

    // Login
    await this.client.login(config.botToken);

    // Initialize conversation store
    this.conversationStore = new ConversationStore();
  }

  private setupEventHandlers(): void {
    this.client.on('ready', () => {
      console.log(`Discord bot ready: ${this.client.user?.tag}`);
    });

    this.client.on('messageCreate', async (message) => {
      // Ignore bot messages
      if (message.author.bot) return;

      // Ignore messages that don't mention the bot (in guilds)
      if (message.guild && !message.mentions.has(this.client.user!)) {
        return;
      }

      // Translate to unified format
      const unifiedMessage = await this.translateIncomingMessage(message);

      // Emit to handler
      await this.messageHandler?.(unifiedMessage);
    });
  }

  private async translateIncomingMessage(
    discordMessage: Message
  ): Promise<UnifiedIncomingMessage> {
    // Extract conversation ID (channel + thread)
    const conversationId = discordMessage.thread
      ? `${discordMessage.channelId}-${discordMessage.thread.id}`
      : discordMessage.channelId;

    return {
      id: generateId(),
      platformMessageId: discordMessage.id,
      content: this.stripMentions(discordMessage.content),
      userId: discordMessage.author.id,
      userName: discordMessage.author.username,
      channelId: discordMessage.channelId,
      threadId: discordMessage.thread?.id,
      conversationId,
      platform: 'discord',
      timestamp: discordMessage.createdAt,
      metadata: {
        guildId: discordMessage.guild?.id,
        guildName: discordMessage.guild?.name,
      },
    };
  }

  async sendMessage(
    message: UnifiedOutgoingMessage,
    context: MessageContext
  ): Promise<SendMessageResult> {
    try {
      const channel = await this.client.channels.fetch(context.channelId);

      if (!channel?.isTextBased()) {
        return { success: false, error: 'Channel is not text-based' };
      }

      // Send message
      const sentMessage = await channel.send({
        content: message.content,
        embeds: message.blocks as any[], // Convert to Discord embeds
      });

      return {
        success: true,
        messageId: sentMessage.id,
        timestamp: sentMessage.createdAt.toISOString(),
      };
    } catch (error) {
      return {
        success: false,
        error: error.message,
      };
    }
  }

  async sendStreamingMessage(
    stream: AsyncIterable<string>,
    context: MessageContext,
    options?: StreamingOptions
  ): Promise<SendMessageResult> {
    // Post initial message
    const initialResponse = await this.sendMessage(
      { content: options?.initialMessage || '...' },
      context
    );

    if (!initialResponse.success) return initialResponse;

    // Get channel and message
    const channel = await this.client.channels.fetch(context.channelId);
    if (!channel?.isTextBased()) {
      return { success: false, error: 'Channel is not text-based' };
    }

    const message = await channel.messages.fetch(initialResponse.messageId!);

    // Stream updates by editing the message
    let accumulated = '';
    let lastUpdate = Date.now();
    const UPDATE_INTERVAL_MS = 1000;

    for await (const chunk of stream) {
      accumulated += chunk;

      // Throttle updates to respect rate limits
      if (Date.now() - lastUpdate >= UPDATE_INTERVAL_MS) {
        // Discord has 2000 char limit per message
        if (accumulated.length > 2000) {
          accumulated = accumulated.slice(0, 1997) + '...';
        }

        await message.edit(accumulated);
        lastUpdate = Date.now();
      }
    }

    // Final update
    await message.edit(accumulated);

    return initialResponse;
  }
}
```

#### Discord-Specific Considerations

1. **Message Content Intent**: Required to read message content (privileged intent for 100+ servers)
2. **Character Limit**: 2000 characters per message
3. **Rate Limits**: 5 requests per second per channel, global rate limit of 50 requests/second
4. **Gateway Connection**: Must maintain heartbeat to keep connection alive
5. **Threads**: Thread IDs are the same as parent message IDs

### 5. Agent Message Handler

Coordinates between adapters and AstroAgent instances.

```typescript
class AgentMessageHandler {
  private agents: Map<string, AstroAgent>;
  private adapters: Map<string, MessagingAdapter>;
  private conversationStore: ConversationStore;

  constructor(
    private config: AgentMessageHandlerConfig
  ) {
    this.agents = new Map();
    this.adapters = new Map();
    this.conversationStore = new ConversationStore();
  }

  // Register an agent
  registerAgent(agentId: string, agent: AstroAgent): void {
    this.agents.set(agentId, agent);
  }

  // Register an adapter
  registerAdapter(platform: string, adapter: MessagingAdapter): void {
    this.adapters.set(platform, adapter);

    // Set up message handler
    adapter.onMessage(async (message) => {
      await this.handleIncomingMessage(message);
    });
  }

  private async handleIncomingMessage(
    message: UnifiedIncomingMessage
  ): Promise<void> {
    try {
      // Get or create conversation context
      const context = await this.getOrCreateContext(message);

      // Route to appropriate agent
      const agent = this.agents.get(context.agentId);
      if (!agent) {
        throw new Error(`No agent found for ID: ${context.agentId}`);
      }

      // Get adapter for response
      const adapter = this.adapters.get(message.platform);
      if (!adapter) {
        throw new Error(`No adapter found for platform: ${message.platform}`);
      }

      // Prepare message context
      const messageContext: MessageContext = {
        platform: message.platform,
        channelId: message.channelId,
        threadId: message.threadId,
        userId: message.userId,
        conversationId: message.conversationId,
      };

      // Stream agent response
      await agent.stream({
        prompt: message.content,
        threadId: message.conversationId,
        userId: message.userId,

        onChunk: async (chunk) => {
          // Accumulated chunks will be sent on finish
        },

        onStepStart: async (step) => {
          // Show "Agent is using tool X" indicator
          if (this.config.showToolUsage) {
            await adapter.sendMessage({
              content: `🔧 Using: ${step.name}`,
              ephemeral: true,
            }, messageContext);
          }
        },

        onStepEnd: async (step) => {
          // Tool completed
        },

        onReasoningStart: async () => {
          // Show thinking indicator
          if (this.config.showReasoning) {
            await adapter.sendMessage({
              content: '🤔 Thinking...',
              ephemeral: true,
            }, messageContext);
          }
        },

        onFinish: async (result) => {
          // Send final response
          await adapter.sendMessage({
            content: result,
            createThread: this.config.autoThreadResponses,
          }, messageContext);

          // Update conversation context
          context.lastMessageAt = new Date();
          context.messageCount++;
          await this.conversationStore.update(context);
        },

        onError: async (error) => {
          // Send error message
          await adapter.sendMessage({
            content: `❌ Error: ${error.message}`,
            ephemeral: this.config.ephemeralErrors,
          }, messageContext);
        },
      });

    } catch (error) {
      console.error('Error handling message:', error);
      // Send user-friendly error
      const adapter = this.adapters.get(message.platform);
      if (adapter) {
        await adapter.sendMessage({
          content: 'Sorry, I encountered an error processing your message.',
          ephemeral: true,
        }, {
          platform: message.platform,
          channelId: message.channelId,
          userId: message.userId,
          conversationId: message.conversationId,
        });
      }
    }
  }

  private async getOrCreateContext(
    message: UnifiedIncomingMessage
  ): Promise<ConversationContext> {
    let context = await this.conversationStore.get(message.conversationId);

    if (!context) {
      context = {
        conversationId: message.conversationId,
        agentId: this.config.defaultAgentId,
        platform: message.platform,
        channelId: message.channelId,
        threadId: message.threadId,
        userId: message.userId,
        createdAt: new Date(),
        lastMessageAt: new Date(),
        messageCount: 0,
        metadata: {},
      };

      await this.conversationStore.create(context);
    }

    return context;
  }
}

interface AgentMessageHandlerConfig {
  defaultAgentId: string;
  autoThreadResponses?: boolean;
  showToolUsage?: boolean;
  showReasoning?: boolean;
  ephemeralErrors?: boolean;
}
```

### 6. Conversation Store

Manages conversation state and context across messages.

```typescript
interface ConversationContext {
  conversationId: string;
  agentId: string;
  platform: string;
  channelId: string;
  threadId?: string;
  userId: string;
  createdAt: Date;
  lastMessageAt: Date;
  messageCount: number;
  metadata: Record<string, unknown>;
}

interface ConversationStore {
  get(conversationId: string): Promise<ConversationContext | null>;
  create(context: ConversationContext): Promise<void>;
  update(context: ConversationContext): Promise<void>;
  delete(conversationId: string): Promise<void>;
  list(filters?: ConversationFilters): Promise<ConversationContext[]>;
}

// Implementation can use Redis, PostgreSQL, or in-memory storage
class RedisConversationStore implements ConversationStore {
  constructor(private redis: Redis) {}

  async get(conversationId: string): Promise<ConversationContext | null> {
    const data = await this.redis.get(`conversation:${conversationId}`);
    return data ? JSON.parse(data) : null;
  }

  async create(context: ConversationContext): Promise<void> {
    await this.redis.setex(
      `conversation:${context.conversationId}`,
      86400 * 7, // 7 days TTL
      JSON.stringify(context)
    );
  }

  async update(context: ConversationContext): Promise<void> {
    await this.create(context); // Upsert
  }

  async delete(conversationId: string): Promise<void> {
    await this.redis.del(`conversation:${conversationId}`);
  }

  async list(filters?: ConversationFilters): Promise<ConversationContext[]> {
    // Implementation depends on indexing strategy
    const keys = await this.redis.keys('conversation:*');
    const contexts = await Promise.all(
      keys.map(async (key) => {
        const data = await this.redis.get(key);
        return data ? JSON.parse(data) : null;
      })
    );
    return contexts.filter(Boolean);
  }
}
```

## Advanced Features

### 1. Streaming Support

Both Slack and Discord support message updates, enabling streaming responses.

**Implementation Strategy:**
- Post initial message immediately
- Accumulate stream chunks
- Update message every 1-2 seconds (respects rate limits)
- Final update with complete response

**User Experience:**
- Users see "..." initially
- Message updates in real-time as agent thinks
- Natural conversation flow

### 2. Threading Strategy

**Slack Threading:**
- Use `thread_ts` to group related messages
- Option 1: Automatically thread all bot responses
- Option 2: Thread only multi-turn conversations
- Option 3: Let user decide with configuration

**Discord Threading:**
- Create thread on first response
- All subsequent messages in same conversation go to thread
- Keeps main channel clean

### 3. Context Management

**Short-term Context:**
- Store in ConversationStore with TTL
- Includes recent message history
- Platform-specific metadata

**Long-term Context:**
- Delegate to Mastra's Memory system
- Agent handles its own memory management
- Adapter only provides conversation ID

### 4. Rate Limiting

**Slack:**
- Tier-based limits (1+ requests/sec for Tier 2-4)
- Message updates count against limits
- Implement token bucket algorithm

**Discord:**
- 5 requests/sec per channel
- 50 requests/sec global
- Use queue with priority

```typescript
class RateLimiter {
  private tokens: number;
  private lastRefill: number;

  constructor(
    private maxTokens: number,
    private refillRate: number // tokens per second
  ) {
    this.tokens = maxTokens;
    this.lastRefill = Date.now();
  }

  async acquire(): Promise<void> {
    while (true) {
      this.refill();

      if (this.tokens >= 1) {
        this.tokens -= 1;
        return;
      }

      // Wait before retrying
      await sleep(100);
    }
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    const newTokens = elapsed * this.refillRate;

    this.tokens = Math.min(this.maxTokens, this.tokens + newTokens);
    this.lastRefill = now;
  }
}
```

### 5. Error Handling

**Categories:**
1. **Platform Errors**: Rate limits, network failures, auth issues
2. **Agent Errors**: Tool failures, timeouts, invalid responses
3. **System Errors**: Database failures, memory issues

**Strategy:**
- Exponential backoff for retries
- User-friendly error messages
- Log errors for debugging
- Optional ephemeral error messages

### 6. Observability

**Metrics to Track:**
- Message volume (incoming/outgoing)
- Response latency
- Error rates by type
- Token usage per conversation
- Active conversations
- Agent tool usage

**Integration Points:**
- OpenTelemetry for tracing
- Prometheus for metrics
- Structured logging
- Platform-specific analytics

```typescript
interface ObservabilityConfig {
  enableTracing?: boolean;
  enableMetrics?: boolean;
  metricsPort?: number;
  tracingEndpoint?: string;
  logLevel?: 'debug' | 'info' | 'warn' | 'error';
}

class ObservabilityManager {
  private tracer?: Tracer;
  private meter?: Meter;

  constructor(config: ObservabilityConfig) {
    if (config.enableTracing) {
      this.setupTracing(config.tracingEndpoint);
    }

    if (config.enableMetrics) {
      this.setupMetrics(config.metricsPort);
    }
  }

  traceMessage(message: UnifiedIncomingMessage): Span {
    return this.tracer!.startSpan('message.process', {
      attributes: {
        'message.platform': message.platform,
        'message.userId': message.userId,
        'message.conversationId': message.conversationId,
      },
    });
  }

  recordMessageProcessed(platform: string): void {
    this.meter!.createCounter('messages.processed').add(1, {
      platform,
    });
  }

  recordLatency(platform: string, latencyMs: number): void {
    this.meter!.createHistogram('message.latency').record(latencyMs, {
      platform,
    });
  }
}
```

## Deployment Architecture

### Option 1: Standalone Service

```
┌────────────────────────────────────────┐
│     Messaging Adapter Service          │
│                                        │
│  ┌──────────────────────────────────┐ │
│  │   Express/Fastify Server         │ │
│  │   - Webhook endpoints            │ │
│  │   - Health checks                │ │
│  │   - Metrics endpoint             │ │
│  └──────────────────────────────────┘ │
│                                        │
│  ┌──────────────────────────────────┐ │
│  │   Adapters                       │ │
│  │   - SlackAdapter                 │ │
│  │   - DiscordAdapter               │ │
│  └──────────────────────────────────┘ │
│                                        │
│  ┌──────────────────────────────────┐ │
│  │   Agent Manager                  │ │
│  │   - Load agents                  │ │
│  │   - Route messages               │ │
│  └──────────────────────────────────┘ │
└────────────────────────────────────────┘
         │                    │
         ▼                    ▼
    ┌────────┐          ┌────────┐
    │ Redis  │          │ Agents │
    └────────┘          └────────┘
```

### Option 2: Serverless (AWS Lambda / Cloudflare Workers)

```
┌─────────────────────────────────────────┐
│          API Gateway / Workers          │
│          - /slack/events                │
│          - /discord/events              │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│     Lambda Function / Worker            │
│     - Parse event                       │
│     - Initialize adapter                │
│     - Process message                   │
│     - Invoke agent                      │
└─────────────────────────────────────────┘
         │                    │
         ▼                    ▼
    ┌────────┐          ┌────────┐
    │DynamoDB│          │ Agent  │
    └────────┘          │ Runtime│
                        └────────┘
```

### Option 3: Container-Based (with Astro CLI)

Integrate with Astro's container infrastructure:

```yaml
# astro-spec.yml
apiVersion: astro.dev/v1
kind: AgentInfrastructure

metadata:
  name: customer-support-agent
  version: 1.0.0

runtime:
  image: myorg/support-agent:latest
  environment:
    - name: ENABLE_SLACK_ADAPTER
      value: "true"
    - name: SLACK_BOT_TOKEN
      valueFrom:
        secretRef: slack-bot-token
    - name: ENABLE_DISCORD_ADAPTER
      value: "true"
    - name: DISCORD_BOT_TOKEN
      valueFrom:
        secretRef: discord-bot-token

# Messaging adapter configuration
messaging:
  adapters:
    - type: slack
      config:
        socketMode: true
        autoThread: true
        showToolUsage: true

    - type: discord
      config:
        autoThread: true
        showReasoning: false

interface:
  type: messaging
  platforms:
    - slack
    - discord

models:
  primary:
    name: gpt-4-turbo
    provider: openai

observability:
  tracing:
    enabled: true
    capture:
      - message_events
      - agent_responses
      - tool_calls
```

## Package Structure

```
packages/astro-messaging/
├── src/
│   ├── core/
│   │   ├── adapter.ts              # Base adapter interface
│   │   ├── message.ts              # Message types
│   │   ├── context.ts              # Context management
│   │   └── handler.ts              # Agent message handler
│   │
│   ├── adapters/
│   │   ├── slack/
│   │   │   ├── adapter.ts          # Slack adapter implementation
│   │   │   ├── translator.ts       # Message translation
│   │   │   ├── formatter.ts        # Slack-specific formatting
│   │   │   └── types.ts            # Slack types
│   │   │
│   │   └── discord/
│   │       ├── adapter.ts          # Discord adapter implementation
│   │       ├── translator.ts       # Message translation
│   │       ├── formatter.ts        # Discord-specific formatting
│   │       └── types.ts            # Discord types
│   │
│   ├── store/
│   │   ├── conversation.ts         # Conversation store interface
│   │   ├── redis.ts                # Redis implementation
│   │   ├── memory.ts               # In-memory implementation
│   │   └── postgres.ts             # PostgreSQL implementation
│   │
│   ├── utils/
│   │   ├── rate-limiter.ts         # Rate limiting
│   │   ├── retry.ts                # Retry logic
│   │   ├── streaming.ts            # Streaming helpers
│   │   └── formatting.ts           # Text formatting
│   │
│   ├── observability/
│   │   ├── tracing.ts              # OpenTelemetry tracing
│   │   ├── metrics.ts              # Prometheus metrics
│   │   └── logging.ts              # Structured logging
│   │
│   └── index.ts                    # Public API
│
├── examples/
│   ├── slack-bot.ts                # Slack bot example
│   ├── discord-bot.ts              # Discord bot example
│   └── multi-platform.ts           # Multi-platform example
│
├── tests/
│   ├── unit/
│   ├── integration/
│   └── e2e/
│
├── package.json
└── README.md
```

## Usage Examples

### Basic Slack Bot

```typescript
import { AstroAgent } from '@astro/agents';
import { SlackAdapter, AgentMessageHandler } from '@astro/messaging';

// Create agent
const agent = new AstroAgent()
  .meta({
    title: 'Support Agent',
    description: 'Customer support assistant',
  })
  .instructions('You are a helpful customer support assistant.')
  .model('openai/gpt-4-turbo');

// Create Slack adapter
const slackAdapter = new SlackAdapter();
await slackAdapter.initialize({
  botToken: process.env.SLACK_BOT_TOKEN!,
  socketMode: true,
  appToken: process.env.SLACK_APP_TOKEN!,
  autoCreateThreads: true,
});

// Create message handler
const handler = new AgentMessageHandler({
  defaultAgentId: 'support-agent',
  showToolUsage: true,
  ephemeralErrors: true,
});

// Register agent and adapter
handler.registerAgent('support-agent', agent);
handler.registerAdapter('slack', slackAdapter);

// Start
await slackAdapter.start();
console.log('Slack bot is running!');
```

### Multi-Platform Bot

```typescript
import { AstroAgent } from '@astro/agents';
import {
  SlackAdapter,
  DiscordAdapter,
  AgentMessageHandler
} from '@astro/messaging';

// Create agent
const agent = new AstroAgent()
  .meta({
    title: 'Assistant',
    description: 'General purpose assistant',
  })
  .instructions('You are a helpful assistant.');

// Create adapters
const slackAdapter = new SlackAdapter();
await slackAdapter.initialize({
  botToken: process.env.SLACK_BOT_TOKEN!,
  socketMode: true,
  appToken: process.env.SLACK_APP_TOKEN!,
});

const discordAdapter = new DiscordAdapter();
await discordAdapter.initialize({
  botToken: process.env.DISCORD_BOT_TOKEN!,
});

// Create handler
const handler = new AgentMessageHandler({
  defaultAgentId: 'assistant',
  autoThreadResponses: true,
});

// Register
handler.registerAgent('assistant', agent);
handler.registerAdapter('slack', slackAdapter);
handler.registerAdapter('discord', discordAdapter);

// Start both
await Promise.all([
  slackAdapter.start(),
  discordAdapter.start(),
]);

console.log('Multi-platform bot is running!');
```

### With Custom Context Routing

```typescript
const handler = new AgentMessageHandler({
  defaultAgentId: 'general',
});

// Register multiple agents
handler.registerAgent('general', generalAgent);
handler.registerAgent('support', supportAgent);
handler.registerAgent('sales', salesAgent);

// Custom routing logic
handler.setRoutingStrategy(async (message) => {
  // Route based on channel
  if (message.channelName?.includes('support')) {
    return 'support';
  }
  if (message.channelName?.includes('sales')) {
    return 'sales';
  }
  return 'general';
});
```

## Configuration

### Environment Variables

```bash
# Slack Configuration
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token  # For Socket Mode
SLACK_SIGNING_SECRET=your-signing-secret

# Discord Configuration
DISCORD_BOT_TOKEN=your-bot-token

# Storage
REDIS_URL=redis://localhost:6379
POSTGRES_URL=postgresql://localhost:5432/astro

# Observability
ENABLE_TRACING=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
ENABLE_METRICS=true
METRICS_PORT=9090

# Behavior
AUTO_CREATE_THREADS=true
SHOW_TOOL_USAGE=true
SHOW_REASONING=false
EPHEMERAL_ERRORS=true

# Rate Limiting
SLACK_RATE_LIMIT_REQUESTS_PER_SECOND=3
DISCORD_RATE_LIMIT_REQUESTS_PER_SECOND=5
```

### Configuration File

```yaml
# config.yaml
adapters:
  slack:
    enabled: true
    socketMode: true
    autoCreateThreads: true
    showToolUsage: true
    rateLimit:
      requestsPerSecond: 3
      burstSize: 10

  discord:
    enabled: true
    autoCreateThreads: true
    showReasoning: false
    rateLimit:
      requestsPerSecond: 5
      burstSize: 20

storage:
  type: redis
  url: ${REDIS_URL}
  ttl: 604800  # 7 days

observability:
  tracing:
    enabled: true
    endpoint: ${OTEL_EXPORTER_OTLP_ENDPOINT}
    sampleRate: 1.0
  metrics:
    enabled: true
    port: 9090
  logging:
    level: info
    format: json

agents:
  default: general-agent
  routing:
    strategy: channel-based
    rules:
      - pattern: ".*-support"
        agent: support-agent
      - pattern: ".*-sales"
        agent: sales-agent
```

## Security Considerations

### 1. Token Management

- Store bot tokens in secure secret management (Vault, AWS Secrets Manager)
- Rotate tokens regularly
- Use environment-specific tokens (dev/staging/prod)
- Never commit tokens to version control

### 2. Message Validation

- Verify Slack request signatures
- Validate Discord gateway messages
- Sanitize user input before processing
- Implement message size limits

### 3. Rate Limiting

- Implement per-user rate limits
- Protect against spam/abuse
- Use exponential backoff for retries

### 4. Data Privacy

- Don't log sensitive user data
- Implement conversation TTL
- Support data deletion requests (GDPR)
- Encrypt conversation data at rest

### 5. Access Control

- Implement channel/guild allowlists
- Support role-based permissions
- Restrict admin commands

## Testing Strategy

### Unit Tests

- Message translation logic
- Formatting functions
- Rate limiter behavior
- Context management

### Integration Tests

- Slack adapter with mock API
- Discord adapter with mock gateway
- Agent message handler
- Conversation store implementations

### End-to-End Tests

- Full message flow (platform → adapter → agent → response)
- Streaming responses
- Error handling
- Thread management

### Load Tests

- Concurrent message handling
- Rate limiter effectiveness
- Memory usage under load
- Database performance

## Performance Optimization

### 1. Connection Pooling

- Reuse HTTP connections for API calls
- Maintain WebSocket connections efficiently
- Pool database connections

### 2. Caching

- Cache user info (username, email)
- Cache channel metadata
- Cache agent configurations

### 3. Message Batching

- Batch conversation store updates
- Batch metric reporting
- Batch log writes

### 4. Async Processing

- Process messages asynchronously
- Use message queues for high load
- Implement circuit breakers

## Migration Path

For existing Slack/Discord bots:

1. **Phase 1: Wrapper**
   - Wrap existing bot with adapter interface
   - Minimal code changes
   - Test in parallel

2. **Phase 2: Translation**
   - Migrate to unified message format
   - Update message handlers
   - Deploy gradually

3. **Phase 3: Integration**
   - Integrate with AgentMessageHandler
   - Add streaming support
   - Enable advanced features

## Future Enhancements

### Additional Platforms

- **Microsoft Teams**: Similar to Slack with Bot Framework
- **Telegram**: Bot API with webhook/polling
- **WhatsApp**: Business API integration
- **Matrix**: Decentralized messaging protocol

### Advanced Features

- **Multi-agent Orchestration**: Route to specialized agents
- **Voice Integration**: Slack/Discord voice channels
- **Rich Media**: Interactive components, modals, forms
- **Analytics Dashboard**: Conversation insights
- **A/B Testing**: Test different agent configurations
- **Human Handoff**: Escalate to human support

### MCP Integration

Expose agents via Model Context Protocol:

```yaml
mcp:
  enabled: true
  expose:
    - messaging-agent
  protocol:
    version: "1.0"
    capabilities:
      - streaming
      - tools
      - memory
```

## References

### Documentation Sources

**Mastra Framework:**
- [About Mastra](https://mastra.ai/docs)
- [Using Agents - Mastra Docs](https://mastra.ai/docs/agents/overview)
- [Agent Reference](https://mastra.ai/en/reference/agents/agent)
- [Mastra GitHub](https://github.com/mastra-ai/mastra)

**Slack API:**
- [The Events API](https://docs.slack.dev/apis/events-api/)
- [Using the Slack Events API](https://api.slack.com/events-api)
- [Sending messages using incoming webhooks](https://api.slack.com/messaging/webhooks)
- [Threading in Slack](https://medium.com/slack-developer-blog/bringing-your-bot-into-threaded-messages-cd272a42924f)
- [Message Threading](https://api.slack.com/docs/message-threading)

**Discord API:**
- [Gateway - Discord Developer Portal](https://discord.com/developers/docs/events/gateway)
- [Threads - discord.js Guide](https://discordjs.guide/popular-topics/threads.html)
- [Gateway Intents](https://discordjs.guide/legacy/popular-topics/intents)

## Conclusion

This messaging adapter layer provides a robust, production-ready foundation for connecting Astro agents to Slack and Discord. The design prioritizes developer experience, reliability, and extensibility while maintaining platform-specific capabilities. The unified interface allows agents to be deployed across multiple platforms with minimal code changes, while the modular architecture makes it easy to add support for additional platforms in the future.
