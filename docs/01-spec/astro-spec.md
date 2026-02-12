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
- Tools: Containerized tool services (e.g., MCP servers)

**Integrations (user provides credentials, platform injects them):**
- Model APIs: Cloud-hosted LLM/embedding APIs (Anthropic, OpenAI, etc.)
- Tool APIs: External API integrations (GitHub, etc.)

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
  # Self-hosted models as containers

knowledge:
  # Self-hosted stores in containers (Qdrant, Redis, Postgres)

tools:
  # Containerized tool services

integrations:
  models:
    # Cloud model APIs (Anthropic, OpenAI, etc.)
  tools:
    # External APIs (GitHub, etc.)

interfaces:
  # User/system interaction points

ingestion:
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
    container:
      image: huggingface/transformers:latest
      gpu: false

  local_llm:
    container:
      image: ollama/ollama:latest
      gpu: true

  reranker:
    container:
      image: vllm/vllm-openai:latest
      gpu: true
```

### knowledge

Knowledge stores provide memory and context to the agent. Use `provider` for platform-managed stores or `container` for custom stores — they are mutually exclusive.

```yaml
knowledge:
  # Provider mode: platform manages image, port, health checks
  docs:
    provider: qdrant
    persistent: true

  cache:
    provider: redis

  local_db:
    provider: postgres
    persistent: true

  # Container mode: user manages everything
  custom_store:
    container:
      image: my-store:latest
      port: 5000
```

**Supported providers:**

| Provider | Image | Port | Mount Path | Health Check | Injected Env Vars |
|----------|-------|------|------------|--------------|-------------------|
| `qdrant` | `qdrant/qdrant:latest` | 6333 (+gRPC 6334) | `/qdrant/storage` | HTTP `/healthz` | `QDRANT_HOST`, `QDRANT_PORT`, `QDRANT_URL` |
| `redis` | `redis:7-alpine` | 6379 | `/data` | `redis-cli ping` | `REDIS_HOST`, `REDIS_PORT`, `REDIS_URL` |
| `postgres` | `postgres:15-alpine` | 5432 | `/var/lib/postgresql/data` | `pg_isready -U postgres` | `POSTGRES_HOST`, `POSTGRES_PORT` |

Container-mode knowledge stores without a provider get generic env vars: `KNOWLEDGE_{NAME}_HOST`, `KNOWLEDGE_{NAME}_PORT`.

### tools

Custom tools that run as containers in the agent infrastructure.

```yaml
tools:
  websearch:
    provider: puppeteer
```

### integrations

Third-party services and APIs requiring user authentication. Declares what the agent needs to authenticate with. User provides credentials at deploy time and the platform injects them into the agent container as environment variables.

```yaml
integrations:
  models:
    - name: primary_llm
      provider: anthropic
      # Injects ANTHROPIC_API_KEY

    - name: fallback_llm
      provider: openai
      # Injects OPENAI_API_KEY

  tools:
    - name: github
      provider: github
      # Injects GITHUB_TOKEN
```

Credentials are passed through as environment variables. Any provider can be declared — the user supplies the credential key/value at deploy time and it is injected into all containers via a Kubernetes Secret.

### interfaces

```yaml
interfaces:
  slack:
    type: slack
    config:
      bot_token: ${SLACK_BOT_TOKEN}
      app_token: ${SLACK_APP_TOKEN}

  web:
    type: web

  custom_service:
    type: custom
    service:
      name: my-adapter
      image: company/my-adapter:latest
      ports: ["9090:9090"]
      environment:
        API_KEY: ${API_KEY}
```

**Types:** slack, web, custom

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

container:
  build:
    context: .
    dockerfile: Dockerfile

models:
  embedder:
    container:
      image: huggingface/transformers:latest
      gpu: false

knowledge:
  docs:
    provider: qdrant
    persistent: true

  cache:
    provider: redis

integrations:
  models:
    - name: primary_llm
      provider: anthropic

  tools:
    - name: github
      provider: github

interfaces:
  slack:
    type: slack
    config:
      bot_token: ${SLACK_BOT_TOKEN}
      app_token: ${SLACK_APP_TOKEN}

  web:
    type: web

ingestion:
  docs_sync:
    container:
      image: my-docs-sync:latest
      environment:
        SOURCE_REPO: company/engineering-docs
        SOURCE_BRANCH: main
        TARGET_COLLECTION: docs
    trigger:
      type: schedule
      schedule: "0 */6 * * *"
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
