# Astro Agent Development Guide

To view the CLI help (installation, quick start, commands) in the terminal, run **`ast docs help`**.

## Quick Start: Complete Agent Example

This is the complete pattern for building an agent with tools and connecting it to messaging:

```typescript
import { AstroAgent } from '@saswatds/astro-agent';
import { Graph, z } from '@saswatds/astro-graph';
import { createMessagingClient } from '@saswatds/astro-messaging';

// 1. Define a tool using Graph
const fetchUrl = new Graph(z.object({ url: z.string() }))
  .meta({
    title: 'Fetch URL',
    toolDescription: 'Use this to retrieve web page content'  // LLM sees this
  })
  .run((f) => f.evaluate({
    fn: async (input) => {
      const response = await fetch(input.url);
      return { content: await response.text() };
    }
  }), { name: 'Fetch' })
  .compile();

// 2. Create agent with tool attached
const agent = new AstroAgent()
  .model('anthropic/claude-sonnet-4-20250514')
  .instructions('You help users research topics. Use Fetch URL to get web content.')
  .tool({ type: 'graph', graph: fetchUrl });  // <-- Attach tool here

// 3. Connect to messaging and handle messages
const client = createMessagingClient();
const stream = await client.connect();

for await (const message of stream) {
  // 4. Map incoming message to agent.stream() parameters
  const reply = await new Promise<string>((resolve) => {
    agent.stream({
      prompt: message.content,                      // User's message text
      threadId: message.conversationId,             // Links to conversation memory
      userId: message.user?.id ?? 'anonymous',      // Links to user context
      onFinish: (result: string) => resolve(result),
      onError: (error: Error) => resolve(`Error: ${error.message}`),
    });
  });

  // 5. Send reply back through the stream
  stream.sendMessage({
    conversationId: message.conversationId,
    content: reply,
    platform: message.platform,
    platformContext: message.platformContext,
  });
}
```

---

## Critical: The `stream()` API

**`agent.stream()` returns `Promise<void>`.** The only way to get the reply is via callbacks.

| What you need | How to get it |
|---------------|---------------|
| Final reply | `onFinish` callback |
| Streaming chunks | `onChunk` callback |
| Errors | `onError` callback |

```typescript
// Use a Promise wrapper to await the reply
const reply = await new Promise<string>((resolve) => {
  agent.stream({
    prompt: message.content,
    threadId: message.conversationId,
    userId: message.user?.id ?? 'anonymous',
    onFinish: (result: string) => resolve(result),
    onError: (error: Error) => resolve(`Error: ${error.message}`),
  });
});

// WRONG - Do NOT do this:
// const result = await agent.stream(...);  // Returns void!
// for await (const chunk of result) { }    // Will fail!
```

---

## Message Mapping Reference

| agent.stream() param | Source | Purpose |
|---------------------|--------|---------|
| `prompt` | `message.content` | The user's message text |
| `threadId` | `message.conversationId` | Links agent memory to this conversation |
| `userId` | `message.user?.id` | Links agent memory to this user |

| Reply field | Value | Purpose |
|-------------|-------|---------|
| `conversationId` | `message.conversationId` | Route reply to correct conversation |
| `content` | `reply` from `onFinish` | The agent's response |
| `platform` | `message.platform` | Preserve platform context |
| `platformContext` | `message.platformContext` | Platform-specific routing info |

---

## Adding Tools to an Agent

Use `.tool({ type: 'graph', graph: myGraph })` to attach a Graph as a tool:

```typescript
const agent = new AstroAgent()
  .model('anthropic/claude-sonnet-4-20250514')
  .instructions('...')
  .tool({ type: 'graph', graph: fetchUrl })      // First tool
  .tool({ type: 'graph', graph: searchDb })      // Second tool
  .tool({ type: 'graph', graph: sendEmail });    // Third tool
```

The `toolDescription` in Graph's `.meta()` tells the LLM when to use the tool:

```typescript
const myTool = new Graph(z.object({ query: z.string() }))
  .meta({
    title: 'Search Database',
    toolDescription: 'Use this to search the database for user records'  // <-- LLM sees this
  })
  .run((f) => f.evaluate({ fn: async (input) => { /* logic */ } }))
  .compile();
```

---

## Streaming Partial Responses (Optional)

Use `onChunk` if you want real-time streaming to the UI:

```typescript
agent.stream({
  prompt: message.content,
  threadId: message.conversationId,
  userId: message.user?.id ?? 'anonymous',
  onChunk: (chunk: string) => {
    stream.sendMessage({
      conversationId: message.conversationId,
      content: chunk,
      platform: message.platform,
      platformContext: message.platformContext,
      isPartial: true,  // Indicates partial response
    });
  },
  onFinish: (result: string) => {
    stream.sendMessage({
      conversationId: message.conversationId,
      content: result,
      platform: message.platform,
      platformContext: message.platformContext,
      isPartial: false,  // Final complete response
    });
  },
});
```

---

## All Callbacks

| Callback | Type | Description |
|----------|------|-------------|
| `onChunk` | `(chunk: string) => void` | Each text chunk as it streams |
| `onFinish` | `(result: string) => void` | Final complete response |
| `onError` | `(error: Error) => void` | Error handling |
| `onStepStart` | `(step: AgentStep) => void` | When agent starts a tool call |
| `onStepEnd` | `(step: AgentStep) => void` | When agent completes a tool call |
| `onReasoningStart` | `() => void` | When reasoning begins |
| `onReasoningChunk` | `(chunk: string) => void` | Reasoning text chunks |
| `onReasoningEnd` | `() => void` | When reasoning completes |

---

## Packages

| Package | Purpose |
|---------|---------|
| `@saswatds/astro-agent` | LLM-driven agent with tools and memory |
| `@saswatds/astro-graph` | Deterministic workflows as composable tools |
| `@saswatds/astro-messaging` | gRPC messaging (always needed) |

---

## Project Structure

```
├── agent/index.ts           # Main agent entry point
├── ingestion/index.ts       # Data ingestion pipeline (batch job)
├── astroai.yml                # Deployment configuration
├── Dockerfile               # Agent container
└── Dockerfile.ingestion     # Ingestion container (if enabled)
```

**Agent** = long-running process handling messages.
**Ingestion** = batch job that runs to completion.

---

## Configuration (`astroai.yml`)

Defines container build, interfaces (web/Slack), model providers, knowledge stores, and ingestion triggers.

---

## Development

```bash
ast dev          # Start agent in dev mode
bun run ingest   # Run ingestion pipeline
```

Environment variables are automatically injected by `ast dev`. See `.env`.

---

## Raw Code Option

If you need to bypass the DSL, implement the messaging interface directly:

```typescript
// Receive message shape
{ conversationId, platform, content, user: { id, username }, platformContext }

// Send reply with same shape
stream.sendMessage({ conversationId, platform, content, platformContext })
```

Use raw code only for performance-critical non-LLM paths or unsupported integrations.
