# Migrating to Streaming Agent Responses

If you scaffolded an agent before the streaming update, your agent sends plain `Message` objects back through the gRPC stream. This works but means the web playground can't stream tokens in real-time and Slack doesn't show status indicators.

This guide walks you through updating to the new `ContentChunk` / `StatusUpdate` protocol.

## What changed

| Before                                  | After                                                         |
| --------------------------------------- | ------------------------------------------------------------- |
| `stream.sendMessage({...})` per token   | `stream.sendContentChunk(convId, { type: 'DELTA', content })` |
| No start/end signals                    | `START` before streaming, `END` with full result              |
| No status updates                       | `StatusUpdate` for thinking, tool use, etc.                   |
| Only `onChunk` / `onFinish` / `onError` | Also `onReasoningStart/End`, `onStepStart/End`                |

## Step-by-step

### 1. Update imports

**Before:**
```typescript
import { AstroAgent } from '@saswatds/astro-agent';
import {
  MessagingClient,
  type AgentConfig,
  type AgentResponse,
  type Message,
} from '@astropods/messaging';
```

**After:**
```typescript
import { AstroAgent } from '@saswatds/astro-agent';
import type { AgentStep } from '@saswatds/astro-types';
import {
  MessagingClient,
  type AgentConfig,
  type AgentResponse,
  type Message,
} from '@astropods/messaging';
```

### 2. Replace the message handler

**Before:**
```typescript
stream.on('response', async (response: AgentResponse) => {
  const message = (response as { incomingMessage?: Message }).incomingMessage;
  if (!message) return;

  const agentUser = {
    id: AGENT_NAME.toLowerCase().replace(/\s+/g, '-'),
    username: AGENT_NAME,
  };

  agent.stream({
    prompt: message.content,
    threadId: message.conversationId,
    userId: message.user?.id ?? 'anonymous',
    onChunk: (chunk: string) => {
      stream.sendMessage({
        conversationId: message.conversationId,
        platform: message.platform,
        platformContext: message.platformContext,
        content: chunk,
        user: agentUser,
      });
    },
    onFinish: () => {
      console.log('Response complete');
    },
    onError: (error: Error) => {
      console.error('Error:', error);
    },
  });
});
```

**After:**
```typescript
stream.on('response', async (response: AgentResponse) => {
  const message = (response as { incomingMessage?: Message }).incomingMessage;
  if (!message) return;

  // Signal start of streaming response
  stream.sendContentChunk(message.conversationId, { type: 'START', content: '' });

  agent.stream({
    prompt: message.content,
    threadId: message.conversationId,
    userId: message.user?.id ?? 'anonymous',
    onReasoningStart: () => {
      stream.sendStatusUpdate(message.conversationId, { status: 'THINKING' });
    },
    onReasoningEnd: () => {
      stream.sendStatusUpdate(message.conversationId, { status: 'GENERATING' });
    },
    onStepStart: (step: AgentStep) => {
      stream.sendStatusUpdate(message.conversationId, {
        status: 'PROCESSING',
        customMessage: `Running ${step.name}`,
        emoji: '🔧',
      });
    },
    onStepEnd: (step: AgentStep) => {
      stream.sendStatusUpdate(message.conversationId, {
        status: 'ANALYZING',
        customMessage: `Finished ${step.name}`,
      });
    },
    onChunk: (chunk: string) => {
      stream.sendContentChunk(message.conversationId, { type: 'DELTA', content: chunk });
    },
    onFinish: (result: string) => {
      stream.sendContentChunk(message.conversationId, { type: 'END', content: '' });
      console.log('Response complete');
    },
    onError: (error: Error) => {
      console.error('Error:', error);
    },
  });
});
```

### 3. Remove the `agentUser` variable

The old pattern required constructing a `User` object and a full `Message` with `platform`, `platformContext`, etc. for every chunk. The new methods only need a `conversationId` — the server handles routing.

You can delete any `agentUser` const that was only used for `sendMessage` calls.

### 4. Update `@astropods/messaging` dependency

Make sure you're on a version that includes the new `sendContentChunk` / `sendStatusUpdate` methods:

```bash
bun update @astropods/messaging @saswatds/astro-types
```

## What each chunk type does

| Type      | When to send                    | Content                 |
| --------- | ------------------------------- | ----------------------- |
| `START`   | Before streaming begins         | Empty string            |
| `DELTA`   | Each LLM token                  | The token text          |
| `END`     | When streaming completes        | Empty string            |
| `REPLACE` | To overwrite a previous message | Full replacement text   |

## What each status does

| Status       | When to send                       | Platform behavior                                     |
| ------------ | ---------------------------------- | ----------------------------------------------------- |
| `THINKING`   | Reasoning/extended thinking starts | Slack: thought balloon emoji. Web: thinking indicator |
| `GENERATING` | Resuming text generation           | Slack: pencil emoji. Web: generating indicator        |
| `PROCESSING` | Tool is executing                  | Slack: gear emoji. Web: processing indicator          |
| `ANALYZING`  | Interpreting tool results          | Slack: chart emoji. Web: analyzing indicator          |
| `SEARCHING`  | Searching knowledge base           | Slack: magnifying glass. Web: searching indicator     |
| `CUSTOM`     | Anything else                      | Uses `customMessage` and optional `emoji`             |

## Minimal migration

If you just want streaming to work without status updates, the minimum change is:

```typescript
// Send START before streaming
stream.sendContentChunk(message.conversationId, { type: 'START', content: '' });

agent.stream({
  prompt: message.content,
  threadId: message.conversationId,
  userId: message.user?.id ?? 'anonymous',
  onChunk: (chunk: string) => {
    stream.sendContentChunk(message.conversationId, { type: 'DELTA', content: chunk });
  },
  onFinish: (result: string) => {
    stream.sendContentChunk(message.conversationId, { type: 'END', content: '' });
  },
  onError: (error: Error) => {
    console.error('Error:', error);
  },
});
```

This gives you real-time streaming in the web playground and proper message delivery in Slack, without the status indicators.
