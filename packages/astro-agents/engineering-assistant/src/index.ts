import { config } from 'dotenv';
import { EngineeringAgent } from './agent';

config();

async function main() {
  const grpcServerAddr = process.env.GRPC_SERVER_ADDR || 'localhost:9090';

  console.log('🚀 Starting Engineering Assistant Agent...');
  console.log(`   gRPC Server: ${grpcServerAddr}`);

  const agent = new EngineeringAgent({
    grpcServerAddr,
    anthropicApiKey: process.env.ANTHROPIC_API_KEY!,
    qdrantHost: process.env.QDRANT_HOST || 'localhost',
    qdrantPort: parseInt(process.env.QDRANT_PORT || '6333'),
    redisHost: process.env.REDIS_HOST || 'localhost',
    redisPort: parseInt(process.env.REDIS_PORT || '6379'),
    embedderHost: process.env.EMBEDDER_HOST || 'localhost',
  });

  await agent.start();

  process.on('SIGINT', async () => {
    console.log('\n\n🛑 Shutting down gracefully...');
    await agent.stop();
    process.exit(0);
  });

  process.on('SIGTERM', async () => {
    console.log('\n\n🛑 Shutting down gracefully...');
    await agent.stop();
    process.exit(0);
  });
}

main().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});
