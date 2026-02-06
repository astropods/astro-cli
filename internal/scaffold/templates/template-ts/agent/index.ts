/**
 * {{.Name}} - {{.Description}}
 *
 * This agent connects to the Astro messaging service via gRPC and responds
 * to incoming messages from any configured interface (web, slack, etc.).
 *
 * Environment variables (automatically injected by 'astro dev'):
 *   GRPC_SERVER_ADDR - Messaging service address (default: localhost:9090)
{{- if eq .Model "anthropic"}}
 *   ANTHROPIC_API_KEY - Anthropic API key for Claude models
{{- else if eq .Model "openai"}}
 *   OPENAI_API_KEY - OpenAI API key for GPT models
{{- end}}
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}
 *   QDRANT_HOST - Qdrant vector database host
 *   QDRANT_PORT - Qdrant vector database port
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}
 *   REDIS_HOST - Redis host
 *   REDIS_PORT - Redis port
{{- end}}
 */

import {
  MessagingClient,
  type AgentResponse,
  type Message,
  type PlatformContext,
} from '@saswatds/astro-messaging';

const AGENT_NAME = '{{.Name}}';
const GRPC_SERVER_ADDR = process.env.GRPC_SERVER_ADDR || 'localhost:9090';

// Create the messaging client
const client = new MessagingClient(GRPC_SERVER_ADDR);

async function handleMessage(response: AgentResponse, stream: any) {
  const { conversationId } = response;

  // Uncomment to debug raw gRPC messages:
  // console.log('🔍 RAW GRPC MESSAGE:');
  // console.log(JSON.stringify(response, null, 2));
  // console.log('---\n');

  // Extract the incoming message from the response payload
  // proto-loader with keepCase:false converts snake_case to camelCase at runtime
  const message = (response as { incomingMessage?: Message }).incomingMessage;

  if (!message) {
    // Not a message event (could be status update, etc.)
    return;
  }

  // Extract user info with fallback for empty strings
  const username = message.user?.username || message.user?.id || 'Anonymous User';
  const displayName = username.trim() === '' ? 'Anonymous User' : username;

  console.log('📨 Received message:');
  console.log('   From:', displayName);
  console.log('   Platform:', message.platform);
  console.log('   Content:', message.content);

  // Get platform context for routing the response back
  const platformContext: PlatformContext | undefined = message.platformContext;

  //
  // TODO: Implement your agent logic here
  //
  // This is where you would:
  // 1. Process the incoming message
  // 2. Call your AI model (Anthropic, OpenAI, etc.)
  // 3. Search your knowledge base
  // 4. Generate a response
  //
{{- if eq .Model "anthropic"}}

  // Example with Anthropic Claude:
  // const anthropic = new Anthropic({ apiKey: process.env.ANTHROPIC_API_KEY });
  // const result = await anthropic.messages.create({
  //   model: 'claude-sonnet-4-20250514',
  //   max_tokens: 1024,
  //   messages: [{ role: 'user', content: message.content }],
  // });
  // const reply = result.content[0].text;
{{- else if eq .Model "openai"}}

  // Example with OpenAI:
  // const response = await fetch('https://api.openai.com/v1/chat/completions', {
  //   method: 'POST',
  //   headers: {
  //     'Content-Type': 'application/json',
  //     'Authorization': `Bearer ${process.env.OPENAI_API_KEY}`,
  //   },
  //   body: JSON.stringify({
  //     model: 'gpt-4o',
  //     messages: [{ role: 'user', content: message.content }],
  //   }),
  // });
  // const data = await response.json();
  // const reply = data.choices[0].message.content;
{{- end}}

  // For now, echo the message back
  const reply = `Hello! You said: "${message.content}". I'm ${AGENT_NAME}, ready to help!`;

  console.log('📤 Sending response...');

  const responseMessage = {
    conversationId,
    platform: message.platform,
    platformContext: platformContext,
    content: reply,
    user: {
      id: AGENT_NAME.toLowerCase().replace(/\s+/g, '-'),
      username: AGENT_NAME,
    },
  };

  // Uncomment to debug outgoing messages:
  // console.log('📤 OUTGOING MESSAGE:');
  // console.log(JSON.stringify(responseMessage, null, 2));
  // console.log('---\n');

  // Send the response back through the stream
  stream.sendMessage(responseMessage);

  console.log('✅ Response sent\n');
}

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

  // Handle incoming messages
  stream.on('response', async (response: AgentResponse) => {
    try {
      await handleMessage(response, stream);
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
