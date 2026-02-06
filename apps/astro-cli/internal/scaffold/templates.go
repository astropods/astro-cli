package scaffold

const astroYmlTemplate = `spec: astro/v1
agent: {{.Name}}

meta:
  version: 0.1.0
  description: {{.Description}}

container:
  build:
    context: .
    dockerfile: Dockerfile
    args:
      NPM_SCOPE: "@saswatds"
      NPM_REGISTRY: "https://npm.pkg.github.com"
    secrets:
      - id: npm_token
        env: GITHUB_PACKAGES_TOKEN
{{- if gt (len .Interfaces) 0}}

interfaces:
{{- range .Interfaces}}
{{- if eq . "web"}}
  web:
    type: web
{{- else if eq . "slack"}}
  slack:
    type: slack
{{- end}}
{{- end}}
{{- end}}
{{- if or (ne .Model "none") (gt (len .Tools) 0)}}

integrations:
{{- if ne .Model "none"}}
  models:
{{- if eq .Model "anthropic"}}
    - name: llm
      provider: anthropic
{{- else if eq .Model "openai"}}
    - name: llm
      provider: openai
{{- end}}
{{- end}}
{{- if gt (len .Tools) 0}}
  tools:
{{- range .Tools}}
{{- if eq . "github"}}
    - name: github
      provider: github
{{- end}}
{{- end}}
{{- end}}
{{- end}}
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both") (eq .Knowledge "kv")}}

knowledge:
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}
  docs:
    type: vector
    provider: qdrant
    container:
      image: qdrant/qdrant:latest
      persistent: true
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}
  cache:
    type: kv
    provider: redis
    container:
      image: redis:7-alpine
{{- end}}
{{- end}}
{{- if ne .Ingestion "none"}}

ingestion:
  data:
    container:
      build:
        context: .
        dockerfile: Dockerfile.ingestion
        args:
          NPM_SCOPE: "@saswatds"
          NPM_REGISTRY: "https://npm.pkg.github.com"
        secrets:
          - id: npm_token
            env: GITHUB_PACKAGES_TOKEN
    trigger:
{{- if eq .Ingestion "schedule"}}
      type: schedule
      schedule: "0 * * * *"  # every hour
{{- else if eq .Ingestion "manual"}}
      type: manual
{{- else if eq .Ingestion "startup"}}
      type: startup
{{- end}}
{{- end}}
`

const dockerfileTemplate = `# Build stage
FROM oven/bun:1 AS builder

WORKDIR /app

# NPM private registry configuration
ARG NPM_SCOPE
ARG NPM_REGISTRY

# Install dependencies
COPY package.json bun.lock* ./
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "${NPM_SCOPE}:registry=${NPM_REGISTRY}" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    bun install && \
    rm -f ~/.npmrc

# Copy source
COPY . .

# Runtime stage
FROM oven/bun:1-slim

WORKDIR /app

# Copy from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/agent ./agent
COPY --from=builder /app/package.json ./

# Run the agent
CMD ["bun", "run", "agent/index.ts"]
`

const dockerfileIngestionTemplate = `# Ingestion container
FROM oven/bun:1 AS builder

WORKDIR /app

# NPM private registry configuration
ARG NPM_SCOPE
ARG NPM_REGISTRY

# Install dependencies
COPY package.json bun.lock* ./
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "${NPM_SCOPE}:registry=${NPM_REGISTRY}" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    bun install && \
    rm -f ~/.npmrc

# Copy source
COPY . .

# Runtime stage
FROM oven/bun:1-slim

WORKDIR /app

# Copy from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/ingestion ./ingestion
COPY --from=builder /app/package.json ./

# Run the ingestion script
CMD ["bun", "run", "ingestion/index.ts"]
`

const packageJsonTemplate = `{
  "name": "{{.Name}}",
  "version": "0.1.0",
  "type": "module",
  "description": "{{.Description}}",
  "scripts": {
    "dev": "bun --watch agent/index.ts",
    "dev:ingest": "bun --watch ingestion/index.ts",
    "start": "bun agent/index.ts",
    "ingest": "bun ingestion/index.ts"
  },
  "main": "./agent/index.ts",
  "dependencies": {
    "@saswatds/astro-messaging": "latest"
  },
  "devDependencies": {
    "@types/bun": "^1.1.14",
    "typescript": "^5.9.3"
  }
}
`

const tsconfigTemplate = `{
  "compilerOptions": {
    "target": "ESNext",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "resolveJsonModule": true,
    "declaration": true,
    "outDir": "./dist"
  },
  "include": ["agent/**/*", "ingestion/**/*"],
  "exclude": ["node_modules", "dist"]
}
`

const envExampleTemplate = `# {{.Name}} Environment Variables
# Copy this file to .env and fill in your values

# GitHub Packages Token (for npm registry and container images)
# Required for pulling private packages from GitHub Packages
GITHUB_PACKAGES_TOKEN=your-github-token-here
{{- if eq .Model "anthropic"}}

# Anthropic API Key
ANTHROPIC_API_KEY=your-api-key-here
{{- end}}
{{- if eq .Model "openai"}}

# OpenAI API Key
OPENAI_API_KEY=your-api-key-here
{{- end}}
{{- range .Tools}}
{{- if eq . "github"}}

# GitHub Integration
GITHUB_TOKEN=your-github-token-here
{{- end}}
{{- end}}
{{- range .Interfaces}}
{{- if eq . "slack"}}

# Slack Integration
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token
{{- end}}
{{- end}}
`

const gitignoreTemplate = `# Environment
.env
.env.local
.env.*.local

# Astro
.astro/

# Dependencies
node_modules/

# Build
dist/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
*.log
logs/

# Bun
bun.lockb
`

const dockerignoreTemplate = `.git
.gitignore
.env
.env.*
.astro/
node_modules/
dist/
.idea/
.vscode/
*.md
*.log
`

const agentIndexTemplate = `/**
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
  //     'Authorization': ` + "`Bearer ${process.env.OPENAI_API_KEY}`" + `,
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
  const reply = ` + "`Hello! You said: \"${message.content}\". I'm ${AGENT_NAME}, ready to help!`" + `;

  console.log('📤 Sending response...');

  const responseMessage = {
    conversationId,
    platform: message.platform,
    platformContext: platformContext,
    content: reply,
    user: {
      id: AGENT_NAME.toLowerCase().replace(/\\s+/g, '-'),
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
      id: AGENT_NAME.toLowerCase().replace(/\\s+/g, '-'),
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
`

const ingestionIndexTemplate = `/**
 * {{.Name}} - Data Ingestion Pipeline
 *
 * This script handles data ingestion into your knowledge stores.
 * Run with: bun run ingest
 *
 * Environment variables available:
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}
 *   QDRANT_URL - Qdrant vector database connection URL
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}
 *   REDIS_URL - Redis key-value store connection URL
{{- end}}
{{- range .Tools}}
{{- if eq . "github"}}
 *   GITHUB_TOKEN - GitHub API token for fetching repository data
{{- end}}
{{- end}}
 */

async function main() {
  console.log("Starting ingestion pipeline for {{.Name}}...");

  //
  // TODO: Implement your data ingestion logic here
  //
  // Common ingestion patterns:
  //
  // 1. Fetch data from external sources (APIs, databases, files)
  // 2. Process and chunk the data for embedding
  // 3. Generate embeddings using your model
  // 4. Store in your knowledge base (vector store, key-value store)
  //
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}

  // Example: Ingest documents into Qdrant
  // const qdrantUrl = process.env.QDRANT_URL;
  //
  // const documents = [
  //   { id: "1", content: "Document content here...", metadata: {} },
  // ];
  //
  // for (const doc of documents) {
  //   // 1. Generate embedding for the document
  //   // 2. Upsert into Qdrant collection
  // }
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}

  // Example: Cache data in Redis
  // const redisUrl = process.env.REDIS_URL;
  //
  // Store frequently accessed data or pre-computed results
{{- end}}

  console.log("Ingestion complete!");
}

main().catch(console.error);
`
