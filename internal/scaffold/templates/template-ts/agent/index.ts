/**
 * {{.Name}} - {{.Description}}
 *
 * This agent connects to the Astro messaging service via gRPC and uses
 * AstroAgent to handle LLM calls, memory, and tool execution.
 *
 * Environment variables (automatically injected by 'astro dev'):
 *   GRPC_SERVER_ADDR - Messaging service address (default: localhost:9090)
{{- if .HasIntegration "anthropic"}}
 *   ANTHROPIC_API_KEY - Anthropic API key for Claude models
{{- end}}
{{- if .HasIntegration "openai"}}
 *   OPENAI_API_KEY - OpenAI API key for GPT models
{{- end}}
{{- if .HasKnowledge "qdrant"}}
 *   QDRANT_HOST - Qdrant vector database host
 *   QDRANT_PORT - Qdrant vector database port
{{- end}}
{{- if .HasKnowledge "redis"}}
 *   REDIS_HOST - Redis host
 *   REDIS_PORT - Redis port
{{- end}}
{{- if .HasKnowledge "neo4j"}}
 *   NEO4J_HOST - Neo4j graph database host
 *   NEO4J_PORT - Neo4j graph database port
{{- end}}
 */

import { AstroAgent } from '@saswatds/astro-agent';
import type { AgentStep } from '@saswatds/astro-types';
import {
  MessagingClient,
  type AgentConfig,
  type AgentResponse,
  type Message,
} from '@saswatds/astro-messaging';

const AGENT_NAME = '{{.Name}}';
const GRPC_SERVER_ADDR = process.env.GRPC_SERVER_ADDR || 'localhost:9090';

// Configure the agent
const agent = new AstroAgent()
{{- if and (ne .ModelProvider "") (ne .Model "")}}
  .model('{{.ModelProvider}}/{{.Model}}')
{{- else if .HasIntegration "anthropic"}}
  .model('anthropic/claude-sonnet-4-5')
{{- else if .HasIntegration "openai"}}
  .model('openai/gpt-4o')
{{- end}}
  .meta({ title: '{{.Name}}', description: '{{.Description}}' })
  .instructions('You are {{.Name}}, a helpful AI assistant. {{.Description}}');

// Create the messaging client
const client = new MessagingClient(GRPC_SERVER_ADDR);

async function main() {
  console.log('🚀 Starting ' + AGENT_NAME + '...');
  console.log('   gRPC Server:', GRPC_SERVER_ADDR);

  // Connect to the messaging service
  console.log('📡 Connecting to messaging service...');
  await client.connect();
  console.log('✓ Connected');

  // Check service health
  const health = await client.healthCheck();
  console.log('✓ Service health:', health.status);

  // Create a bidirectional conversation stream
  console.log('🌊 Creating conversation stream...');
  const stream = client.createConversationStream();

  // Send agent config so the playground can display it
  stream.sendAgentConfig(agent.getConfig() as AgentConfig);
  console.log('✓ Agent config sent');

  // Handle incoming messages
  stream.on('response', async (response: AgentResponse) => {
    const message = (response as { incomingMessage?: Message }).incomingMessage;
    if (!message) return;

    const username = message.user?.username || message.user?.id || 'Anonymous User';
    console.log(`📨 ${username}: ${message.content}`);

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
        stream.sendContentChunk(message.conversationId, { type: 'END', content: result });
        console.log('📤 Response complete');
      },
      onError: (error: Error) => {
        console.error('❌ Error handling message:', error);
      },
    });
  });

  stream.on('error', (error: Error) => {
    console.error('❌ Stream error:', error);
  });

  stream.on('end', () => {
    console.log('Stream ended');
  });

  // Register the agent
  console.log('📝 Registering agent...');
  stream.sendMessage({
    conversationId: 'agent-registration',
    platform: 'grpc',
    content: 'Agent ready',
    user: {
      id: AGENT_NAME.toLowerCase().replace(/\s+/g, '-'),
      username: AGENT_NAME,
    },
  });

  console.log('✅ ' + AGENT_NAME + ' is ready and listening for messages!\n');
}

// Graceful shutdown
process.on('SIGINT', () => {
  console.log('\n🛑 Shutting down...');
  client.close();
  process.exit(0);
});

process.on('SIGTERM', () => {
  console.log('\n🛑 Shutting down...');
  client.close();
  process.exit(0);
});

// Start the agent
main().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});
