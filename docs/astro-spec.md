# Astro Spec

The Astro Spec is a YAML-based declarative document that defines the topology of an agent. It specifies a container image that runs the agent, infrastructure resources it needs (self-hosted models, knowledge stores, tools), and third-party integrations requiring user credentials (cloud APIs). The agent container implements its own inference logic. The astro cli reads the spec to build the container image using docker and publishes it to an OCI registry along with the spec. The deployment server uses the spec to provision infrastructure and inject user-configured credentials for integrations.

## Who writes the Astro Spec?
The astro spec is written by the user who is building the agent. The astro plaform provides a bunch of pre-built components like LLMs, Vector Stores, Toolkits etc that can be used to build the agent. For example if a user want to build an agent which is personal assistant over slack which can answer questions from a knowledge base stored in a vector store and mistral 7b is used as the LLM, the user will write an astro spec which uses the slack adapter, a vector store component and mistral 7b component to build the agent.

In the astro spect the user can define the topology of the agent by specifying how the components are connected to each other. The user can also specify configuration parameters for each component like the model name for the LLM, the connection details for the vector store etc. The slack adapter would run as the entrypoint of the agent and would receive messages from slack, pass them to the LLM after fetching relevant context from the vector store and then send the response back to slack.

The astro spec is then used by the astro cli to build the container image and publish it to an OCI registry along with the spec. The deployment server can then read the spec from the OCI registry to deploy the agent by building the necessary infrastructure like VMs, Kubernetes pods, serverless functions etc based on the topology defined in the spec.


## What is not included in the Astro Spec?

The astro spec does not include any information about the deployment environment like cloud provider, region, VPC details etc. This information is provided separately to the deployment server when deploying the agent. The astro spec only defines the topology of the agent and the resources it needs to run. The deployment server is responsible for provisioning the necessary infrastructure based on the topology defined in the astro spec and the deployment environment details provided separately.

**Deployment concerns NOT in spec:** resources (cpu/memory/gpu), guardrails, observability, rate limits, budgets, security policies.

## What is included in the Astro Spec?
The astro spec includes the following information:

**Self-hosted components (deployed as containers):**
- Container image details: The base image to use for the agent, any additional packages or dependencies to install etc.
- Models: Local LLMs, embedding models, or rerankers that run in containers
- Knowledge stores: Vector databases, Redis, Postgres, etc. that run in containers
- Tools: Function implementations in your codebase, MCP servers, builtin tools requiring containers

**Integrations (platform manages user credentials and injects them):**
- Model APIs: Cloud-hosted LLM/embedding APIs (Anthropic, OpenAI, Cohere, etc.)
- Knowledge services: Managed services (Pinecone, MongoDB Atlas, AWS RDS, etc.)
- Tool APIs: External API integrations (GitHub API, Slack API, web search, Jira, etc.)

**Agent configuration:**
- Ingestion definitions: Cron or event-based data collection/ingestion pipelines
- Interface definitions: User interaction channels (Slack, HTTP APIs, etc.)

Note: User wouldn't be able to define anything and everyting, it must be supported by the astro platform. For example, if the astro platform does not support a particular LLM or vector store, the user would not be able to use it in the astro spec.

---

## Top-Level Structure

```yaml
spec: astro/v1
agent: <name>

meta:
  version: <semver>
  description: <string>
  tags: [<string>]

container:
  # How to build/run the agent

models:
  # Self-hosted models needing containers (Ollama, vLLM, etc.)

knowledge:
  # Self-hosted stores in containers (Qdrant, Redis, Postgres)

tools:
  # Custom functions, MCP servers, builtin tools

integrations:
  models:
    # Cloud model APIs requiring user credentials (Anthropic, OpenAI, etc.)
  knowledge:
    # Managed services requiring user credentials (Pinecone, MongoDB Atlas, etc.)
  tools:
    # External APIs requiring user credentials (GitHub, Slack, web search, etc.)

interfaces:
  # User/system interaction points

injections:
  # Data pipelines to update knowledge
```

---

## Section Details

### meta

```yaml
meta:
  version: 1.0.0
  description: Knowledge assistant for engineering docs
  tags: [support, internal, engineering]
  owner: platform-team
  repo: github.com/company/agent
```

### container

```yaml
container:
  # Option 1: Pre-built image
  image: registry.io/agent:v1

  # Option 2: Build from source
  build:
    context: .
    dockerfile: Dockerfile
    target: production
    args:
      NODE_ENV: production

  healthcheck:
    path: /health
    interval: 30s
    timeout: 5s
```

### models

Self-hosted models that run as containers in the agent infrastructure.

```yaml
models:
  embedder:
    provider: sentence-transformers
    model: all-MiniLM-L6-v2
    config:
      dimensions: 384
    container:
      image: huggingface/transformers:latest
      gpu: false

  local_llm:
    provider: ollama
    model: mistral:7b
    config:
      context_window: 8192
    container:
      image: ollama/ollama:latest
      gpu: true

  reranker:
    provider: vllm
    model: BAAI/bge-reranker-v2-m3
    container:
      image: vllm/vllm-openai:latest
      gpu: true
```

**Supported providers:** ollama, vllm, sentence-transformers, huggingface

### knowledge

Self-hosted knowledge stores that run as containers in the agent infrastructure.

```yaml
knowledge:
  docs:
    type: vector
    provider: qdrant
    config:
      collection: engineering-docs
      dimensions: 384
      metric: cosine
    embedding: models.embedder  # ref to self-hosted model
    container:
      image: qdrant/qdrant:latest
      persistent: true

  cache:
    type: kv
    provider: redis
    config:
      ttl: 3600
    container:
      image: redis:7-alpine

  local_db:
    type: sql
    provider: postgres
    config:
      database: agent_data
    container:
      image: postgres:15-alpine
      persistent: true
```

**Supported types:** vector, kv, document, sql, graph
**Supported providers:** qdrant, redis, postgres, sqlite, mongo

### tools

Custom tools that are part of the agent codebase or require infrastructure deployment.

```yaml
tools:
  custom_calculator:
    type: function
    config:
      runtime: node
      handler: tools/calculator.js
      functions:
        - name: calculate_metrics
          description: Calculate business metrics
          parameters:
            type: object
            properties:
              metric_type: {type: string}
              date_range: {type: string}

  data_processor:
    type: function
    config:
      runtime: python
      handler: tools/processor.py
      functions:
        - name: process_data
          description: Process and transform data

  mcp_server:
    type: mcp
    config:
      name: internal-tools
      container:
        image: company/mcp-tools:latest
        port: 8081
      tools:
        - database_query
        - file_operations

  code_sandbox:
    type: builtin/code-interpreter
    config:
      runtime: python
      timeout: 30s
      sandbox: true
    container:
      image: astro/code-interpreter:latest
```

**Supported types:** function (custom code), mcp (MCP servers), builtin/* (platform-provided tools)

### integrations

Third-party services and APIs requiring user authentication. Declares what the agent needs to authenticate with and required permissions/scopes. Platform provides authentication UI for users to connect their accounts (OAuth, API keys, etc.), then injects credentials into the agent container at runtime.

```yaml
integrations:
  models:
    primary_llm:
      provider: anthropic
      # Platform prompts user to authenticate and injects ANTHROPIC_API_KEY

    fallback_llm:
      provider: openai
      # Platform prompts user to authenticate and injects OPENAI_API_KEY

  knowledge:
    vector_store:
      provider: pinecone
      # Platform prompts user to authenticate and injects PINECONE_API_KEY

    conversations:
      provider: mongodb-atlas
      # Platform prompts user to authenticate and injects MONGODB_URI

  tools:
    github:
      provider: github
      scopes:
        - repo:read
        - issues:write
        - issues:read
      # Platform prompts user for OAuth with required scopes

    slack:
      provider: slack
      scopes:
        - chat:write
        - channels:read
      # Platform prompts user for OAuth with required scopes

    web_search:
      provider: tavily
      # Platform prompts user to authenticate and injects TAVILY_API_KEY
```

**Supported model providers:** anthropic, openai, mistral, cohere, google
**Supported knowledge providers:** pinecone, weaviate, mongodb-atlas, dynamodb
**Supported tool providers:** github, slack, jira, web-search (tavily, serper), etc.

### interfaces

```yaml
interfaces:
  slack:
    type: messaging/slack
    config:
      bot_token: ${SLACK_BOT_TOKEN}
      app_token: ${SLACK_APP_TOKEN}
      events: [message, app_mention, reaction_added]
      auto_thread: true

  api:
    type: http
    config:
      port: 8080
      auth:
        type: api_key
        header: X-API-Key
        keys: ${API_KEYS}
      cors:
        origins: ["https://app.company.com"]

  discord:
    type: messaging/discord
    config:
      bot_token: ${DISCORD_TOKEN}
      guild_ids: [123456789]

  webhook:
    type: http/webhook
    config:
      path: /webhooks/incoming
      secret: ${WEBHOOK_SECRET}
      events: [push, pull_request]
```

**Types:** http, http/webhook, messaging/slack, messaging/discord, messaging/teams, grpc, websocket

### ingestion

```yaml
ingestion:
  docs_sync:
    container:
      image: my-ingest-worker:latest
      environment:
        SOURCE_REPO: company/docs
        SOURCE_BRANCH: main
        TARGET_COLLECTION: docs
    trigger:
      type: schedule
      schedule: "0 */4 * * *"  # every 4 hours

  api_sync:
    container:
      build:
        context: ./ingestion
        dockerfile: Dockerfile
      environment:
        API_URL: https://api.internal.com/knowledge
    trigger:
      type: schedule
      schedule: "0 0 * * *"  # daily

  initial_load:
    container:
      image: my-bootstrap-worker:latest
      environment:
        TARGET_COLLECTION: docs
    trigger:
      type: startup  # runs once automatically at deploy time

  on_demand_reindex:
    container:
      image: my-reindex-worker:latest
      environment:
        TARGET_COLLECTION: docs
    trigger:
      type: manual  # triggered via API: POST /api/v1/deployments/:name/:version/ingestion/on_demand_reindex/trigger

  event_listener:
    container:
      image: my-webhook-receiver:latest
      port: 8080
      environment:
        TARGET_COLLECTION: docs
    trigger:
      type: webhook  # deploys as long-running Deployment + Service + Ingress
```

Each ingestion entry is a container that runs on a trigger. The container owns its own logic (fetching, chunking, embedding, upserting) and configuration via environment variables.

**Trigger types:**
- `schedule` — cron expression, creates a CronJob
- `startup` — runs a one-shot Job automatically at deploy time
- `manual` — runs a one-shot Job on demand via API endpoint
- `webhook` — deploys a long-running Deployment + Service (+ Ingress) that listens for incoming HTTP calls from external systems

---

## Complete Example

```yaml
spec: astro/v1
agent: engineering-assistant

meta:
  version: 2.0.0
  description: Engineering knowledge assistant with self-hosted and external components
  tags: [engineering, support, internal]
  owner: platform-team

container:
  build:
    context: .
    dockerfile: Dockerfile

models:
  embedder:
    provider: sentence-transformers
    model: all-MiniLM-L6-v2
    config:
      dimensions: 384
    container:
      image: huggingface/transformers:latest
      gpu: false

knowledge:
  docs:
    type: vector
    provider: qdrant
    config:
      dimensions: 384
      metric: cosine
    embedding: models.embedder
    container:
      image: qdrant/qdrant:latest
      persistent: true

  cache:
    type: kv
    provider: redis
    config:
      ttl: 3600
    container:
      image: redis:7-alpine

tools:
  doc_search:
    type: function
    config:
      runtime: python
      handler: tools/search.py
      functions:
        - name: search_docs
          description: Search internal documentation

integrations:
  models:
    primary_llm:
      provider: anthropic

  tools:
    github:
      provider: github
      scopes:
        - repo:read
        - issues:read

    web_search:
      provider: tavily

interfaces:
  slack:
    type: messaging/slack
    config:
      bot_token: ${SLACK_BOT_TOKEN}
      app_token: ${SLACK_APP_TOKEN}

  api:
    type: http
    config:
      port: 8080

injections:
  docs_sync:
    source:
      type: github
      config:
        repo: company/engineering-docs
        branch: main
        paths: ["**/*.md"]
        token: ${GITHUB_TOKEN}
    trigger:
      type: schedule
      cron: "0 */6 * * *"
    pipeline:
      - step: chunk
        config: {strategy: semantic, max_size: 1000}
      - step: embed
        model: models.embedder
      - step: upsert
        target: knowledge.docs
```

---

## Design Principles

1. **Build vs Deploy separation** - Spec defines agent topology; deployment server handles resources, guardrails, observability
2. **Infrastructure not logic** - Spec declares what to deploy (containers) and what to connect to (APIs), not how the agent processes requests (inference logic lives in agent code)
3. **Self-hosted vs Integrations** - Clear separation in spec structure:
   - `models`, `knowledge`, `tools` sections: Self-hosted components (deployed as containers)
   - `integrations` section: Third-party services requiring user credentials (platform manages and injects)
4. **Credential management** - All third-party services declared in `integrations` so platform can provide configuration UI and inject user credentials
5. **Named references** - Components defined once, referenced by name (e.g., `models.embedder`, `integrations.models.primary_llm`)
6. **Flat structure** - Top-level sections for each concern, no deep nesting
7. **Declarative** - Describe what, not how; platform handles orchestration
8. **Defaults** - Sensible defaults; minimal config for simple agents
9. **Extensible** - config maps allow new options without schema changes
10. **Environment injection** - ${VAR} syntax for user-provided secrets
11. **Container-based ingestion** - Opaque containers for data processing
