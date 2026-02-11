/**
 * obs-agent - An AI-powered agent with observability
 *
 * This agent connects to the Astro messaging service via gRPC and uses
 * AstroAgent to handle LLM calls, memory, and tool execution.
 *
 * Observability is auto-configured by AstroAgent when OTEL_EXPORTER_OTLP_ENDPOINT
 * is set. No manual OTel setup needed.
 *
 * Environment variables (automatically injected by 'astro dev'):
 *   GRPC_SERVER_ADDR              - Messaging service address (default: localhost:9090)
 *   OPENAI_API_KEY                - OpenAI API key for GPT models
 *   OTEL_EXPORTER_OTLP_ENDPOINT  - OTel collector OTLP endpoint (enables tracing)
 */

import { AstroAgent } from '@saswatds/astro-agent';
import {
  MessagingClient,
  type AgentConfig,
  type AgentResponse,
  type Message,
} from '@saswatds/astro-messaging';
import { workflows } from '@saswatds/astro-workflows';
const { fetchGithubReadme } = workflows;

const AGENT_NAME = 'obs-agent';
const GRPC_SERVER_ADDR = process.env.GRPC_SERVER_ADDR || 'localhost:9090';

// Observability is auto-configured when OTEL_COLLECTOR_URL is set
const agent = new AstroAgent()
  .model('openai/gpt-4o')
  .meta({ title: 'obs-agent', description: 'An AI-powered agent' })
  .instructions('You are obs-agent, a helpful AI assistant. An AI-powered agent')
  .tool({ type: 'graph', graph: fetchGithubReadme.compile() });

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

    try {
      // Stream the message through the agent
      const reply = await new Promise<string>((resolve, reject) => {
        agent.stream({
          prompt: message.content,
          threadId: message.conversationId,
          userId: message.user?.id ?? 'anonymous',
          onFinish: (result: string) => resolve(result),
          onError: (error: Error) => reject(error),
        });
      });

      // Send the reply back through the stream
      stream.sendMessage({
        conversationId: message.conversationId,
        platform: message.platform,
        platformContext: message.platformContext,
        content: reply,
        user: {
          id: AGENT_NAME.toLowerCase().replace(/\s+/g, '-'),
          username: AGENT_NAME,
        },
      });

      console.log('📤 Response sent');
    } catch (error) {
      console.error('❌ Error handling message:', error);
    }
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
