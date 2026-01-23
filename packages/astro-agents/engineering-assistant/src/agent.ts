import {
  MessagingClient,
  Helpers,
  type AgentResponse,
  type Message,
  type ThreadHistoryResponse,
} from '@astro/messaging-sdk-ts';
import Anthropic from '@anthropic-ai/sdk';
import { QdrantClient } from '@qdrant/js-client-rest';
import { createClient, type RedisClientType } from 'redis';

export interface AgentConfig {
  grpcServerAddr: string;
  anthropicApiKey: string;
  qdrantHost: string;
  qdrantPort: number;
  redisHost: string;
  redisPort: number;
  embedderHost: string;
}

export class EngineeringAgent {
  private messagingClient: MessagingClient;
  private anthropic: Anthropic;
  private qdrant: QdrantClient;
  private redis: RedisClientType;
  private stream: any;
  private isRunning: boolean = false;

  constructor(private config: AgentConfig) {
    this.messagingClient = new MessagingClient(config.grpcServerAddr);

    this.anthropic = new Anthropic({
      apiKey: config.anthropicApiKey,
    });

    this.qdrant = new QdrantClient({
      url: `http://${config.qdrantHost}:${config.qdrantPort}`,
    });

    this.redis = createClient({
      url: `redis://${config.redisHost}:${config.redisPort}`,
    });
  }

  async start() {
    console.log('📡 Connecting to messaging service...');
    await this.messagingClient.connect();
    console.log('✓ Connected to messaging service');

    console.log('📦 Connecting to Redis...');
    await this.redis.connect();
    console.log('✓ Connected to Redis');

    console.log('🔍 Checking Qdrant connection...');
    const collections = await this.qdrant.getCollections();
    console.log(`✓ Connected to Qdrant (${collections.collections.length} collections)`);

    console.log('🏥 Checking service health...');
    const health = await this.messagingClient.healthCheck();
    console.log(`✓ Service health: ${health.status}`);

    console.log('🌊 Creating conversation stream...');
    this.stream = this.messagingClient.createConversationStream();

    this.stream.on('response', async (response: AgentResponse) => {
      await this.handleMessage(response);
    });

    this.stream.on('error', (error: Error) => {
      console.error('❌ Stream error:', error);
    });

    this.stream.on('end', () => {
      console.log('Stream ended');
      this.isRunning = false;
    });

    // Send initial registration message
    console.log('📝 Registering agent with messaging service...');
    this.stream.sendMessage({
      conversationId: 'agent-stream',
      platform: 'grpc',
      content: 'Agent ready',
      user: {
        id: 'engineering-assistant',
        username: 'Engineering Assistant',
      },
    });

    this.isRunning = true;
    console.log('✅ Engineering Assistant is ready and listening for messages!\n');
  }

  private async handleMessage(response: AgentResponse) {
    const { conversationId } = response;

    try {
      // Handle incoming platform messages
      // Note: protobuf oneofs put fields directly on the response object
      const message = (response as any).incoming_message;

      if (message) {
        console.log(`\n📨 Received message from ${message.user?.username}`);
        console.log(`   Platform: ${message.platform}`);
        console.log(`   Content: "${message.content.substring(0, 50)}..."`);

        // Access platform_context using snake_case (protobuf field name)
        const platformContext = (message as any).platform_context;

        const cacheKey = `response:${message.content}`;
        const cachedResponse = await this.redis.get(cacheKey);

        if (cachedResponse) {
          console.log('💾 Using cached response');

          console.log('📤 Sending cached response back to platform');
          console.log('   platformContext:', JSON.stringify(platformContext, null, 2));
          this.stream.sendMessage({
            conversationId,
            platform: message.platform,
            platform_context: platformContext,
            content: cachedResponse,
            user: {
              id: 'engineering-assistant',
              username: 'Engineering Assistant',
            },
          });

          console.log('✅ Cached message sent successfully\n');
          return;
        }

        console.log('🤔 Status: THINKING');

        const history = await this.messagingClient.getThreadHistory(conversationId, 10);
        console.log(`📚 Context: ${history.messages.length} previous messages`);

        console.log('🔍 Searching knowledge base...');
        const searchResults = await this.searchDocs(message.content);
        console.log(`   Found ${searchResults.length} relevant documents`);

        console.log('✍️  Status: GENERATING');

        console.log('🤖 Calling Claude API...');
        const answer = await this.generateResponse(message.content, searchResults, history);
        console.log(`   Generated response (${answer.length} chars)`);

        await this.redis.set(cacheKey, answer, {
          EX: 3600,
        });

        console.log(`📤 Sending response to ${message.platform} | channel=${platformContext?.channel_id || 'unknown'} thread=${platformContext?.thread_id || platformContext?.message_id || 'none'}`);
        this.stream.sendMessage({
          conversationId,
          platform: message.platform,
          platform_context: platformContext,
          content: answer,
          user: {
            id: 'engineering-assistant',
            username: 'Engineering Assistant',
          },
        });

        const prompts = this.generateSuggestedPrompts(message.content);
        console.log(`💡 Suggested prompts: ${prompts.length}`);

        console.log('✅ Message processed successfully\n');
      } else {
        console.log('⏭️  Received non-message response (status update, etc.)');
      }
    } catch (error) {
      console.error('❌ Error handling message:', error);
    }
  }

  private async generateEmbedding(text: string): Promise<number[]> {
    try {
      const response = await fetch(`http://${this.config.embedderHost}:8000/embed`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ texts: [text] }),
      });

      if (!response.ok) {
        throw new Error(`Embedder returned status ${response.status}`);
      }

      const result = (await response.json()) as { embeddings: number[][] };
      return result.embeddings[0];
    } catch (error) {
      console.error('Error generating embedding:', error);
      throw error;
    }
  }

  private async searchDocs(query: string): Promise<Array<{ title: string; content: string; score: number }>> {
    try {
      const queryEmbedding = await this.generateEmbedding(query);

      const searchResults = await this.qdrant.search('engineering-docs', {
        vector: queryEmbedding,
        limit: 5,
        with_payload: true,
      });

      return searchResults.map((result: any) => ({
        title: result.payload?.title || 'Untitled',
        content: result.payload?.content || '',
        score: result.score,
      }));
    } catch (error) {
      console.error('Error searching docs:', error);
      return [];
    }
  }

  private async generateResponse(
    query: string,
    searchResults: Array<{ title: string; content: string; score: number }>,
    history: ThreadHistoryResponse
  ): Promise<string> {
    const contextDocs = searchResults
      .map(
        (doc, i) =>
          `## Document ${i + 1}: ${doc.title} (relevance: ${doc.score.toFixed(2)})\n${doc.content}`
      )
      .join('\n\n');

    const conversationContext = history.messages
      .map((msg) => `${msg.user.username}: ${msg.content}`)
      .join('\n');

    const systemPrompt = `You are an engineering assistant helping with Postman-related questions. Use the following documentation to answer questions accurately:

${contextDocs}

Previous conversation:
${conversationContext}

Provide clear, helpful answers based on the documentation. If you're not sure, say so.`;

    try {
      const message = await this.anthropic.messages.create({
        model: 'claude-sonnet-4-20250514',
        max_tokens: 1024,
        system: systemPrompt,
        messages: [
          {
            role: 'user',
            content: query,
          },
        ],
      });

      const textContent = message.content.find((block) => block.type === 'text');
      return textContent && 'text' in textContent ? textContent.text : 'I apologize, but I could not generate a response.';
    } catch (error) {
      console.error('Error calling Claude API:', error);
      return 'I apologize, but I encountered an error while generating a response.';
    }
  }

  private generateSuggestedPrompts(query: string): Array<{ title: string; message: string }> {
    return [
      { title: 'Tell me more', message: 'Can you elaborate on that?' },
      { title: 'Example', message: 'Can you show me an example?' },
      { title: 'Related topics', message: 'What else should I know about this?' },
    ];
  }

  async stop() {
    console.log('Stopping agent...');
    this.isRunning = false;

    if (this.stream) {
      this.stream.end();
    }

    this.messagingClient.close();
    await this.redis.quit();

    console.log('✓ Agent stopped');
  }
}
