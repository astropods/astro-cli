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
{{- if ne .Interface "none"}}

interfaces:
{{- if eq .Interface "http"}}
  api:
    type: http
{{- else if eq .Interface "slack"}}
  messaging:
    type: slack
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
    bun install --frozen-lockfile && \
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
    bun install --frozen-lockfile && \
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
{{- if eq .Interface "slack"}}

# Slack Integration
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token
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
 * This is your Astro agent's entry point. When running with 'astro dev',
 * the following environment variables are automatically injected:
 *
 *   PORT - Server port (default: 8080)
{{- if eq .Model "anthropic"}}
 *   ANTHROPIC_API_KEY - Anthropic API key for Claude models
{{- else if eq .Model "openai"}}
 *   OPENAI_API_KEY - OpenAI API key for GPT models
{{- end}}
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}
 *   QDRANT_URL - Qdrant vector database connection URL
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}
 *   REDIS_URL - Redis key-value store connection URL
{{- end}}
{{- range .Tools}}
{{- if eq . "github"}}
 *   GITHUB_TOKEN - GitHub API token for repository access
{{- end}}
{{- end}}
{{- if eq .Interface "slack"}}
 *   SLACK_BOT_TOKEN - Slack bot OAuth token
 *   SLACK_APP_TOKEN - Slack app-level token for Socket Mode
{{- end}}
 */

// Create an HTTP server using Bun's built-in server
// Astro will route requests to this server based on your interface config
const server = Bun.serve({
  port: process.env.PORT || 8080,

  async fetch(req) {
    const url = new URL(req.url);

    // Health check endpoint - used by Astro to verify the agent is running
    if (url.pathname === "/health") {
      return Response.json({ status: "healthy" });
    }

    // Main agent endpoint - receives incoming requests
    if (req.method === "POST" && url.pathname === "/") {
      const body = await req.json();

      //
      // TODO: Implement your agent logic here
      //
      // Example: Parse the incoming message and generate a response
      // The request body typically contains:
      //   - message: The user's input text
      //   - context: Additional context from the conversation
      //
{{- if eq .Model "anthropic"}}

      // Access your Anthropic API key:
      // const apiKey = process.env.ANTHROPIC_API_KEY;
      //
      // Example: Call Claude API
      // const response = await fetch("https://api.anthropic.com/v1/messages", {
      //   method: "POST",
      //   headers: {
      //     "Content-Type": "application/json",
      //     "x-api-key": apiKey,
      //     "anthropic-version": "2023-06-01",
      //   },
      //   body: JSON.stringify({
      //     model: "claude-sonnet-4-20250514",
      //     max_tokens: 1024,
      //     messages: [{ role: "user", content: body.message }],
      //   }),
      // });
{{- else if eq .Model "openai"}}

      // Access your OpenAI API key:
      // const apiKey = process.env.OPENAI_API_KEY;
      //
      // Example: Call OpenAI API
      // const response = await fetch("https://api.openai.com/v1/chat/completions", {
      //   method: "POST",
      //   headers: {
      //     "Content-Type": "application/json",
      //     "Authorization": ` + "`Bearer ${apiKey}`" + `,
      //   },
      //   body: JSON.stringify({
      //     model: "gpt-4o",
      //     messages: [{ role: "user", content: body.message }],
      //   }),
      // });
{{- end}}
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}

      // Connect to Qdrant vector store:
      // const qdrantUrl = process.env.QDRANT_URL;
      // Use for semantic search over your knowledge base
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}

      // Connect to Redis cache:
      // const redisUrl = process.env.REDIS_URL;
      // Use for caching responses or storing conversation state
{{- end}}

      const response = {
        message: "Hello from {{.Name}}!",
        received: body,
      };

      return Response.json(response);
    }

    // Return 404 for unhandled routes
    return new Response("Not Found", { status: 404 });
  },
});

console.log("{{.Name}} listening on http://localhost:" + server.port);
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
