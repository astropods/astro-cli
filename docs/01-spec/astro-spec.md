# Astro Spec

The Astro Spec is a YAML-based declarative document that defines the topology of an agent. It specifies a container image that runs the agent and the infrastructure resources it needs — both self-hosted components (containers) and cloud APIs (credential injection). The agent container implements its own inference logic. The astro cli reads the spec to build the container image using docker and publishes it to an OCI registry along with the spec. The deployment server uses the spec to provision infrastructure and inject user-configured credentials.

## Who writes the Astro Spec?
The astro spec is written by the user who is building the agent. The astro plaform provides a bunch of pre-built components like LLMs, Vector Stores, Toolkits etc that can be used to build the agent. For example if a user want to build an agent which is personal assistant over slack which can answer questions from a knowledge base stored in a vector store and mistral 7b is used as the LLM, the user will write an astro spec which uses the slack adapter, a vector store component and mistral 7b component to build the agent.

In the astro spect the user can define the topology of the agent by specifying how the components are connected to each other. The user can also specify configuration parameters for each component like the model name for the LLM, the connection details for the vector store etc. The slack adapter would run as the entrypoint of the agent and would receive messages from slack, pass them to the LLM after fetching relevant context from the vector store and then send the response back to slack.

The astro spec is then used by the astro cli to build the container image and publish it to an OCI registry along with the spec. The deployment server can then read the spec from the OCI registry to deploy the agent by building the necessary infrastructure like VMs, Kubernetes pods, serverless functions etc based on the topology defined in the spec.


## What is not included in the Astro Spec?

The astro spec does not include any information about the deployment environment like cloud provider, region, VPC details etc. This information is provided separately to the deployment server when deploying the agent. The astro spec only defines the topology of the agent and the resources it needs to run. The deployment server is responsible for provisioning the necessary infrastructure based on the topology defined in the astro spec and the deployment environment details provided separately.

**Deployment concerns NOT in spec:** resources (cpu/memory), guardrails, observability, rate limits, budgets, security policies, interfaces (slack, web), ingestion cron schedules.

## What is included in the Astro Spec?
The astro spec includes the following information:

**Components (self-hosted or cloud, declared in their natural section):**
- Container image details: The base image to use for the agent, any additional packages or dependencies to install etc.
- Models: LLMs, embedding models, rerankers — self-hosted containers (ollama, vLLM) or cloud APIs (Anthropic, OpenAI)
- Knowledge stores: Vector databases, caches, relational stores — self-hosted containers (Qdrant, Redis, Postgres) or cloud APIs (Pinecone)
- Tools: Tool services — self-hosted containers (Puppeteer, MCP servers) or cloud APIs (GitHub, GitLab)

**Agent configuration:**
- Ingestion definitions: Event-based data collection/ingestion pipelines (trigger type declared; schedule provided at deploy time or in `dev` section)

Note: User wouldn't be able to define anything and everyting, it must be supported by the astro platform. For example, if the astro platform does not support a particular LLM or vector store, the user would not be able to use it in the astro spec.

---

## Top-Level Structure

```yaml
spec: astro/v1
name: <name>

meta:
  version: <semver>
  description: <string>
  tags: [<string>]

agent:
  # How to build/run the agent

models:
  # LLMs, embedders, rerankers — self-hosted or cloud

knowledge:
  # Data stores — self-hosted or cloud

tools:
  # Tool services — self-hosted or cloud

integrations:
  # Custom credential injection for external services

ingestion:
  # Data pipelines to update knowledge

dev:
  # Local development overrides (interfaces, schedules)
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

### agent

```yaml
agent:
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

Models the agent uses. Each entry uses one of three modes — they are mutually exclusive:
- **Provider (self-hosted):** Platform deploys a container. Use for models that run locally (e.g. `ollama`).
- **Provider (cloud):** Platform injects credentials. Use for cloud model APIs (e.g. `anthropic`, `openai`).
- **Container:** User manages image, port, GPU config. Use for custom model containers.

The platform's provider registry determines whether a provider is self-hosted (has image/port/healthcheck) or cloud (has credential suffixes). The spec author just writes `provider: <name>` — the platform handles the rest.

```yaml
models:
  # Self-hosted provider: platform deploys container
  local_llm:
    provider: ollama

  # Cloud provider: platform injects credentials
  primary:
    provider: anthropic
    # → ANTHROPIC_API_KEY injected

  fallback:
    provider: openai
    # → FALLBACK_API_KEY injected (key name from entry name, suffix from provider)

  # Container mode: user manages everything
  embedder:
    container:
      image: huggingface/transformers:latest

  reranker:
    container:
      image: vllm/vllm-openai:latest
      gpu:
        vram: 24Gi
```

**Supported model providers:**

| Provider | Kind | Image | Port | Health Check | Credential Suffix |
|----------|------|-------|------|--------------|-------------------|
| `ollama` | self-hosted | `ollama/ollama:latest` | 11434 | HTTP `/api/tags` | — |
| `anthropic` | cloud | — | — | — | `API_KEY` |
| `openai` | cloud | — | — | — | `API_KEY` |
| `google` / `gemini` | cloud | — | — | — | `API_KEY` |
| `cohere` | cloud | — | — | — | `API_KEY` |

Self-hosted providers deploy a container; env var `{UPPER(NAME)}_HOST` is injected. Cloud providers inject `{UPPER(NAME)}_{SUFFIX}` as a credential the user provides at deploy time.

Container-mode models get generic env vars: `{UPPER(NAME)}_HOST`.

### knowledge

Knowledge stores provide memory and context to the agent. Same three modes as models: provider (self-hosted), provider (cloud), or container.

```yaml
knowledge:
  # Self-hosted providers: platform deploys containers
  docs:
    provider: qdrant
    persistent: true

  cache:
    provider: redis

  local_db:
    provider: postgres
    persistent: true

  # Cloud provider: platform injects credentials
  vectors:
    provider: pinecone
    # → VECTORS_API_KEY injected

  # Container mode: user manages everything
  custom_store:
    container:
      image: my-store:latest
      port: 5000
```

**Supported knowledge providers:**

| Provider | Kind | Image | Port | Mount Path | Health Check | Injected Env Vars |
|----------|------|-------|------|------------|--------------|-------------------|
| `qdrant` | self-hosted | `qdrant/qdrant:latest` | 6333 (+gRPC 6334) | `/qdrant/storage` | HTTP `/healthz` | `QDRANT_HOST`, `QDRANT_PORT`, `QDRANT_URL` |
| `redis` | self-hosted | `redis:7-alpine` | 6379 | `/data` | `redis-cli ping` | `REDIS_HOST`, `REDIS_PORT`, `REDIS_URL` |
| `postgres` | self-hosted | `postgres:15-alpine` | 5432 | `/var/lib/postgresql/data` | `pg_isready -U postgres` | `POSTGRES_HOST`, `POSTGRES_PORT` |
| `pinecone` | cloud | — | — | — | — | `{NAME}_API_KEY` |

Container-mode knowledge stores without a provider get generic env vars: `KNOWLEDGE_{NAME}_HOST`, `KNOWLEDGE_{NAME}_PORT`.

### tools

Tool services the agent uses. Same three modes as models: provider (self-hosted), provider (cloud), or container.

```yaml
tools:
  # Self-hosted provider
  websearch:
    provider: puppeteer

  # Cloud providers: platform injects credentials
  github:
    provider: github
    # → GITHUB_TOKEN injected

  gitlab:
    provider: gitlab
    # → GITLAB_TOKEN injected
```

**Supported tool providers:**

| Provider | Kind | Credential Suffix |
|----------|------|-------------------|
| `puppeteer` | self-hosted | — |
| `github` | cloud | `TOKEN` |
| `gitlab` | cloud | `TOKEN` |

### integrations

For external services that don't fit into models, knowledge, or tools. Each entry declares the credentials it needs via a `credentials` array.

```yaml
integrations:
  my-service:
    credentials:
      - suffix: API_KEY
        description: API key for my-service
      - suffix: SECRET
        description: Shared secret for HMAC signing
        optional: true
```

This produces env vars `MY_SERVICE_API_KEY` (required) and `MY_SERVICE_SECRET` (optional). At least one credential entry is required. Env var naming follows the same `{UPPER(name)}_{suffix}` convention.

### Credential injection (how it works)

Cloud providers and integrations require user-provided credentials. The platform handles this uniformly:

1. **Spec declares provider** — the platform knows which credentials are needed from its provider registry.
2. **Env var naming** — `{UPPER(entry_name)}_{provider_suffix}`. Entry name is the YAML map key, suffix comes from the provider registry.
3. **Deploy-time collection** — user supplies credential values when deploying.
4. **Injection** — platform injects credentials into all containers via Kubernetes Secrets.

Examples:
- `models.anthropic` with `provider: anthropic` → `ANTHROPIC_API_KEY`
- `models.fallback` with `provider: openai` → `FALLBACK_API_KEY`
- `tools.github` with `provider: github` → `GITHUB_TOKEN`
- `knowledge.vectors` with `provider: pinecone` → `VECTORS_API_KEY`
- `integrations.my-service` → defined by `credentials` array

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
      type: schedule  # schedule expression provided at deploy time or in dev section

  api_sync:
    container:
      build:
        context: ./ingestion
        dockerfile: Dockerfile
      environment:
        API_URL: https://api.internal.com/knowledge
    trigger:
      type: schedule

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
- `schedule` — creates a CronJob; the cron expression is supplied at deploy time (or in the `dev` section for local development)
- `startup` — runs a one-shot Job automatically at deploy time
- `manual` — runs a one-shot Job on demand via API endpoint
- `webhook` — deploys a long-running Deployment + Service (+ Ingress) that listens for incoming HTTP calls from external systems

### dev

Local development overrides read by `astro dev`. Interfaces and cron schedules are deployment concerns — operators choose them at deploy time. The `dev` section provides defaults for local development.

```yaml
dev:
  interfaces: [slack, web]
  schedules:
    docs_sync: "0 */4 * * *"
    api_sync: "0 0 * * *"
```

- `interfaces` — list of interface types to start locally (e.g. `slack`, `web`). Slack credentials (`SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`) are required when `slack` is listed.
- `schedules` — map of ingestion name to cron expression, used for `schedule`-type triggers during local dev.

---

## Complete Example

```yaml
spec: astro/v1
name: engineering-assistant

meta:
  version: 2.0.0
  description: Engineering knowledge assistant with self-hosted and cloud components
  tags: [engineering, support, internal]

agent:
  build:
    context: .
    dockerfile: Dockerfile

models:
  local_llm:
    provider: ollama

  primary:
    provider: anthropic

  embedder:
    container:
      image: huggingface/transformers:latest

knowledge:
  docs:
    provider: qdrant
    persistent: true

  cache:
    provider: redis

tools:
  github:
    provider: github

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

dev:
  interfaces: [slack, web]
  schedules:
    docs_sync: "0 */6 * * *"
```

---

## Design Principles

1. **Build vs Deploy separation** — Spec defines agent topology; deployment server handles resources, guardrails, observability
2. **Infrastructure not logic** — Spec declares what to deploy and what to connect to, not how the agent processes requests (inference logic lives in agent code)
3. **Unified provider model** — Self-hosted and cloud providers live in the same section (`models`, `knowledge`, `tools`). The platform's provider registry determines whether a provider deploys a container or injects credentials. Spec authors don't need to reason about this distinction.
4. **Credential management** — Cloud providers and integrations declared inline; platform enumerates required credentials, provides configuration UI, and injects values at deploy time
5. **Named references** — Components defined once, referenced by name (e.g., `models.primary`, `tools.github`)
6. **Flat structure** — Top-level sections for each concern, no deep nesting
7. **Declarative** — Describe what, not how; platform handles orchestration
8. **Defaults** — Sensible defaults; minimal config for simple agents
9. **Extensible** — config maps allow new options without schema changes
10. **Environment injection** — Platform auto-injects credentials and connection details as env vars; no explicit variable references needed in spec
11. **Container-based ingestion** — Opaque containers for data processing
