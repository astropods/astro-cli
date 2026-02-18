# AstroAI Spec (astro/v1)

**Version:** 1.0
**Date:** 2026-02-17
**Status:** Draft

## Abstract

The AstroAI Spec defines a declarative YAML format for describing the topology of an AI agent — its container, model dependencies, knowledge stores, tool services, integrations, and data ingestion pipelines. The spec is consumed by build tools and deployment servers; it intentionally excludes runtime, orchestration, and deployment-environment concerns.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Introduction

An AstroAI Spec file (`astroai.yml`) is a YAML document that declares:

- The agent's container image (pre-built or build-from-source).
- Sidecar components the agent depends on — models, knowledge stores, and tools — each supplied by either a platform-managed provider or a user-managed container.
- Cloud integrations that require credential injection.
- Data ingestion pipelines with trigger semantics.
- Local development overrides.

The spec does **not** cover: resource limits (CPU/memory), guardrails, observability, rate limits, budgets, security policies, deployment region, or interface routing (Slack, web). These are deployment-time concerns configured separately.

The document format is YAML. Implementations MUST accept files named `astroai.yml` or `astroai.yaml`.

---

## 2. Top-Level Structure

A conforming document MUST contain the following top-level fields:

| Field          | Type                       | Required | Description                                  |
| -------------- | -------------------------- | -------- | -------------------------------------------- |
| `spec`         | string                     | REQUIRED | Spec version identifier. MUST be `astro/v1`. |
| `name`         | string                     | REQUIRED | Unique agent name.                           |
| `meta`         | object                     | REQUIRED | Agent metadata.                              |
| `agent`        | object                     | REQUIRED | Main agent container definition.             |
| `models`       | map\<string, Model\>       | OPTIONAL | Model sidecar entries.                       |
| `knowledge`    | map\<string, Knowledge\>   | OPTIONAL | Knowledge store entries.                     |
| `tools`        | map\<string, Tool\>        | OPTIONAL | Tool service entries.                        |
| `integrations` | map\<string, Integration\> | OPTIONAL | Cloud integration entries.                   |
| `ingestion`    | map\<string, Ingestion\>   | OPTIONAL | Data ingestion pipeline entries.             |
| `dev`          | object                     | OPTIONAL | Local development overrides.                 |

Map keys serve as entry names and are used in credential injection (see [Section 8](#8-credential-injection-model)).

### 2.1 Meta

| Field         | Type     | Required | Description                       |
| ------------- | -------- | -------- | --------------------------------- |
| `description` | string   | OPTIONAL | Human-readable agent description. |
| `tags`        | string[] | OPTIONAL | Classification tags.              |

---

## 3. Agent

The `agent` object defines the main agent container.

| Field         | Type        | Required    | Description                          |
| ------------- | ----------- | ----------- | ------------------------------------ |
| `image`       | string      | Conditional | Pre-built container image reference. |
| `build`       | BuildConfig | Conditional | Build-from-source configuration.     |
| `healthcheck` | Healthcheck | OPTIONAL    | Health check configuration.          |

An agent entry MUST specify exactly one of `image` or `build`. Providing both or neither is invalid.

### 3.1 BuildConfig

| Field        | Type                  | Required | Description                             |
| ------------ | --------------------- | -------- | --------------------------------------- |
| `context`    | string                | REQUIRED | Build context path.                     |
| `dockerfile` | string                | REQUIRED | Path to Dockerfile relative to context. |
| `target`     | string                | OPTIONAL | Multi-stage build target.               |
| `args`       | map\<string, string\> | OPTIONAL | Build arguments passed to the builder.  |
| `secrets`    | BuildSecret[]         | OPTIONAL | Build-time secrets.                     |

#### BuildSecret

| Field | Type   | Required | Description                                              |
| ----- | ------ | -------- | -------------------------------------------------------- |
| `id`  | string | REQUIRED | Secret identifier used in `--mount=type=secret,id=<id>`. |
| `env` | string | OPTIONAL | Environment variable to source the secret value from.    |

### 3.2 Healthcheck

Applies to the agent container and to any `ContainerConfig.healthcheck` in component sections.

| Field      | Type     | Required | Description                                                        |
| ---------- | -------- | -------- | ------------------------------------------------------------------ |
| `test`     | string[] | OPTIONAL | Custom health check command (e.g. `["CMD", "redis-cli", "ping"]`). |
| `path`     | string   | OPTIONAL | HTTP path for health check (auto-generates a `curl` test command). |
| `interval` | string   | OPTIONAL | Check interval. Default: `10s`.                                    |
| `timeout`  | string   | OPTIONAL | Response timeout. Default: `5s`.                                   |
| `retries`  | integer  | OPTIONAL | Consecutive failures before unhealthy. Default: `3`.               |

Implementations SHOULD support both `test` (exec-based) and `path` (HTTP-based) health checks. When `path` is provided, the implementation SHOULD generate an equivalent `test` command.

---

## 4. Component Sections: Models, Knowledge, Tools

Models, knowledge stores, and tools share a unified provider model. Each entry operates in exactly one of two modes:

- **Provider mode** — the entry specifies a `provider` string. The platform resolves this to either a self-hosted provider (deploys a container from its registry) or a cloud provider (injects credentials).
- **Container mode** — the entry specifies a `container` object. The user manages the image, port, and configuration.

These modes are **mutually exclusive**: an entry MUST specify exactly one of `provider` or `container`. Providing both or neither is invalid.

### 4.1 Models

Each entry in the `models` map:

| Field       | Type            | Required    | Description                                                                             |
| ----------- | --------------- | ----------- | --------------------------------------------------------------------------------------- |
| `provider`  | string          | Conditional | Platform-managed provider name (e.g. `ollama`, `anthropic`).                            |
| `model`     | string          | OPTIONAL    | Provider-specific model identifier (e.g. `llama3.2`). Only meaningful in provider mode. |
| `container` | ContainerConfig | Conditional | Custom container configuration.                                                         |

### 4.2 Knowledge

Each entry in the `knowledge` map:

| Field        | Type            | Required    | Description                                                         |
| ------------ | --------------- | ----------- | ------------------------------------------------------------------- |
| `provider`   | string          | Conditional | Platform-managed provider name (e.g. `qdrant`, `pinecone`).         |
| `container`  | ContainerConfig | Conditional | Custom container configuration.                                     |
| `persistent` | boolean         | OPTIONAL    | Whether data SHOULD be persisted across restarts. Default: `false`. |

When `persistent` is `true`, the platform SHOULD provision durable storage for the entry regardless of mode.

### 4.3 Tools

Each entry in the `tools` map:

| Field       | Type            | Required    | Description                                               |
| ----------- | --------------- | ----------- | --------------------------------------------------------- |
| `provider`  | string          | Conditional | Platform-managed provider name (e.g. `github`, `gitlab`). |
| `container` | ContainerConfig | Conditional | Custom container configuration.                           |

### 4.4 ContainerConfig

Used by container-mode entries and by ingestion containers.

| Field         | Type                  | Required    | Description                                                   |
| ------------- | --------------------- | ----------- | ------------------------------------------------------------- |
| `image`       | string                | Conditional | Container image reference.                                    |
| `build`       | BuildConfig           | Conditional | Build-from-source configuration (same schema as Section 3.1). |
| `port`        | integer               | OPTIONAL    | Primary port the container listens on.                        |
| `environment` | map\<string, string\> | OPTIONAL    | Environment variables injected into the container.            |
| `gpu`         | GPUConfig             | OPTIONAL    | GPU resource requirements.                                    |
| `persistent`  | boolean               | OPTIONAL    | Whether data SHOULD be persisted. Default: `false`.           |
| `healthcheck` | Healthcheck           | OPTIONAL    | Health check configuration (same schema as Section 3.2).      |

A ContainerConfig SHOULD specify at least one of `image` or `build`.

#### GPUConfig

| Field     | Type   | Required | Description                                                    |
| --------- | ------ | -------- | -------------------------------------------------------------- |
| `vram`    | string | OPTIONAL | GPU memory required (e.g. `24Gi`).                             |
| `runtime` | string | OPTIONAL | GPU runtime. MUST be one of `cuda` or `rocm`. Default: `cuda`. |

---

## 5. Integrations

The `integrations` section declares external services that do not fit into models, knowledge, or tools. Each integration entry defines the credentials it requires.

| Field         | Type               | Required | Description                                               |
| ------------- | ------------------ | -------- | --------------------------------------------------------- |
| `config`      | map\<string, any\> | OPTIONAL | Provider-specific configuration.                          |
| `credentials` | CustomCredential[] | REQUIRED | Credential requirements. MUST contain at least one entry. |

#### CustomCredential

| Field         | Type    | Required | Description                                                                |
| ------------- | ------- | -------- | -------------------------------------------------------------------------- |
| `suffix`      | string  | REQUIRED | Credential suffix used in env var naming (see Section 8).                  |
| `description` | string  | OPTIONAL | Human-readable description for deploy-time credential prompts.             |
| `optional`    | boolean | OPTIONAL | If `true`, the credential MAY be omitted at deploy time. Default: `false`. |

---

## 6. Ingestion

The `ingestion` section declares data ingestion pipelines. Each entry is a container that runs on a trigger.

| Field       | Type             | Required | Description                            |
| ----------- | ---------------- | -------- | -------------------------------------- |
| `container` | ContainerConfig  | REQUIRED | Container that performs the ingestion. |
| `trigger`   | IngestionTrigger | REQUIRED | When the container runs.               |

#### IngestionTrigger

| Field  | Type   | Required | Description                                                 |
| ------ | ------ | -------- | ----------------------------------------------------------- |
| `type` | string | REQUIRED | MUST be one of: `schedule`, `startup`, `manual`, `webhook`. |

Trigger type semantics:

- **`schedule`** — runs on a cron schedule. The cron expression is supplied at deploy time or via `dev.schedules` for local development.
- **`startup`** — runs once automatically at deploy time.
- **`manual`** — runs on demand via API invocation.
- **`webhook`** — deploys as a long-running service that receives incoming HTTP requests. The container SHOULD declare a `port` when using this trigger type.

---

## 7. Dev

The `dev` section provides local development overrides consumed by `astro dev`. These fields are deployment concerns that do not belong in the normative agent topology.

| Field        | Type                  | Required | Description                                                                        |
| ------------ | --------------------- | -------- | ---------------------------------------------------------------------------------- |
| `interfaces` | string[]              | OPTIONAL | Messaging interfaces to enable locally (e.g. `slack`, `web`).                      |
| `schedules`  | map\<string, string\> | OPTIONAL | Map of ingestion entry name to cron expression for local `schedule`-type triggers. |
| `command`    | string                | OPTIONAL | Custom start command for the agent. Default: `bun --watch run start`.              |

---

## 8. Environment Variable Injection Model

The platform automatically injects environment variables into the agent container to wire it to its dependencies. The injection model differs by entry mode: cloud providers inject credentials, self-hosted providers inject connection details, and container-mode entries inject generic connection details.

### 8.1 Cloud Provider Credentials

Cloud providers (in `models`, `knowledge`, `tools`) require user-provided credentials at deploy time. The env var key is derived from the **provider name**, not the entry name:

**Single entry for a provider:**

```
{UPPER(provider)}_{suffix}
```

Example: one `anthropic` model entry → `ANTHROPIC_API_KEY`.

**Multiple entries for the same provider (duplicate handling):**

Each entry gets a name-qualified key:

```
{UPPER(provider)}_{UPPER(entry_name)}_{suffix}
```

Additionally, a "primary" entry also receives the bare `{UPPER(provider)}_{suffix}` key for convenience. The primary entry is the one whose name matches the provider (e.g. an entry named `anthropic` using `provider: anthropic`); if no entry name matches, the first alphabetically is primary.

When the entry name equals the provider name, the redundant qualified form (e.g. `ANTHROPIC_ANTHROPIC_API_KEY`) is omitted — only the bare key is produced.

Examples (single entry):
- `models.primary` with `provider: anthropic` → `ANTHROPIC_API_KEY`
- `tools.github` with `provider: github` → `GITHUB_TOKEN`
- `knowledge.vectors` with `provider: pinecone` → `PINECONE_API_KEY`

Examples (duplicate entries, two anthropic models):
- `models.anthropic` + `models.sonnet` both with `provider: anthropic`:
  - `anthropic` (name matches provider, primary) → `ANTHROPIC_API_KEY`
  - `sonnet` → `ANTHROPIC_SONNET_API_KEY`

Cloud provider credentials are always required.

### 8.2 Self-Hosted Provider Connection Details

Self-hosted providers deploy a container. The platform injects connection env vars using the provider's env prefix:

**Single entry for a provider:**

```
{EnvPrefix}_HOST, {EnvPrefix}_PORT, {EnvPrefix}_URL
```

Example: one `qdrant` knowledge entry → `QDRANT_HOST`, `QDRANT_PORT`, `QDRANT_URL`.

**Multiple entries for the same self-hosted provider:**

Each entry gets name-qualified keys; the first alphabetically also gets bare keys:

```
{EnvPrefix}_{UPPER(entry_name)}_HOST  (plus bare {EnvPrefix}_HOST for first)
```

Model providers additionally inject `{EnvPrefix}_BASE_URL` (with `/api` appended) and `{EnvPrefix}_MODEL` when a model name is specified.

### 8.3 Container-Mode Connection Details

Container-mode entries (no provider) receive generic section-prefixed env vars:

- **Models:** `MODEL_{UPPER(name)}_HOST`, `MODEL_{UPPER(name)}_PORT`, `MODEL_{UPPER(name)}_URL`
- **Knowledge:** `KNOWLEDGE_{UPPER(name)}_HOST`, `KNOWLEDGE_{UPPER(name)}_PORT`
- **Tools:** `TOOL_{UPPER(name)}_HOST`, `TOOL_{UPPER(name)}_PORT`, `TOOL_{UPPER(name)}_URL`

### 8.4 Integration Credentials

Integration credentials use the **entry name** as prefix:

```
{UPPER(entry_name)}_{suffix}
```

Where `suffix` comes from the `credentials[].suffix` field. Integration credentials are required unless the credential's `optional` field is `true`.

Example: `integrations.my-service` with `credentials: [{suffix: API_KEY}, {suffix: SECRET, optional: true}]` → `MY-SERVICE_API_KEY` (required), `MY-SERVICE_SECRET` (optional).

### 8.5 Name Sanitization

Entry names used in env var keys are sanitized: converted to lowercase, underscores and dots replaced with hyphens, non-alphanumeric characters removed, then uppercased for the env var. For example, entry name `my_model` sanitizes to `my-model`, then uppercases to `MY-MODEL`.

---

## 9. Validation Rules

Implementations MUST enforce the following validation rules:

1. `spec` MUST be a non-empty string.
2. `name` MUST be a non-empty string.
3. `agent` MUST specify exactly one of `image` or `build`. If `build` is present, `build.context` and `build.dockerfile` are REQUIRED.
4. For each entry in `models`: `provider` and `container` are mutually exclusive. Exactly one MUST be present.
5. For each entry in `knowledge`: `provider` and `container` are mutually exclusive. Exactly one MUST be present.
6. For each entry in `tools`: `provider` and `container` are mutually exclusive. Exactly one MUST be present.
7. For each entry in `integrations`: `credentials` MUST be present and MUST contain at least one element. Each credential MUST have a non-empty `suffix`.
8. For each entry in `ingestion`: both `container` and `trigger` are REQUIRED. `trigger.type` MUST be one of `schedule`, `startup`, `manual`, `webhook`.
9. When a `BuildConfig` is provided (in `agent.build`, `container.build`), `context` and `dockerfile` are REQUIRED.
10. When `gpu.runtime` is provided, it MUST be one of `cuda` or `rocm`.

---

## Appendix A: Provider Registries (Non-Normative)

The following tables document the platform's built-in provider registries as of this specification version. Implementations MAY extend these registries.

### A.1 Model Providers

#### Self-Hosted

| Provider | Image                  | Port  | Health Check     | GPU | Default Env                                   |
| -------- | ---------------------- | ----- | ---------------- | --- | --------------------------------------------- |
| `ollama` | `ollama/ollama:latest` | 11434 | HTTP `/api/tags` | Yes | `OLLAMA_HOST=0.0.0.0`, `OLLAMA_KEEP_ALIVE=-1` |

When `model` is specified for a self-hosted provider, the platform sets `{ENV_PREFIX}_MODEL` (e.g. `OLLAMA_MODEL=llama3.2`).

#### Cloud

| Provider    | Credential Suffix | Description                                           |
| ----------- | ----------------- | ----------------------------------------------------- |
| `anthropic` | `API_KEY`         | Anthropic API key for Claude models                   |
| `openai`    | `API_KEY`         | OpenAI API key for GPT models                         |
| `google`    | `API_KEY`         | Google API key for Gemini models                      |
| `gemini`    | `API_KEY`         | Google API key for Gemini models (alias for `google`) |
| `cohere`    | `API_KEY`         | Cohere API key for language models                    |

### A.2 Knowledge Providers

#### Self-Hosted

| Provider   | Image                  | Port | Extra Ports | Mount Path                 | Health Check             | Default Env       |
| ---------- | ---------------------- | ---- | ----------- | -------------------------- | ------------------------ | ----------------- |
| `qdrant`   | `qdrant/qdrant:latest` | 6333 | gRPC 6334   | `/qdrant/storage`          | HTTP `/healthz`          | —                 |
| `redis`    | `redis:7-alpine`       | 6379 | —           | `/data`                    | `redis-cli ping`         | —                 |
| `postgres` | `postgres:15-alpine`   | 5432 | —           | `/var/lib/postgresql/data` | `pg_isready -U postgres` | —                 |
| `neo4j`    | `neo4j:5-community`    | 7474 | Bolt 7687   | `/data`                    | HTTP `/`                 | `NEO4J_AUTH=none` |

#### Cloud

| Provider   | Credential Suffix | Description                          |
| ---------- | ----------------- | ------------------------------------ |
| `pinecone` | `API_KEY`         | Pinecone API key for vector database |

### A.3 Tool Providers

#### Cloud

| Provider | Credential Suffix | Description                 |
| -------- | ----------------- | --------------------------- |
| `github` | `TOKEN`           | GitHub token for API access |
| `gitlab` | `TOKEN`           | GitLab token for API access |

---

## Appendix B: JSON Schema

A machine-readable JSON Schema for this specification is maintained at `astroai.schema.json` in the `astro-spec` package. The schema is generated from the normative type definitions and MAY be used for editor autocompletion and pre-validation.

Schema ID: `https://astromode.ai/schema/astroai.json`

---

## Appendix C: Complete Example

```yaml
spec: astro/v1
name: engineering-assistant

meta:
  description: Engineering knowledge assistant with self-hosted and cloud components
  tags: [engineering, support, internal]

agent:
  build:
    context: .
    dockerfile: Dockerfile
    secrets:
      - id: npm_token
        env: GITHUB_PACKAGES_TOKEN

models:
  local_llm:
    provider: ollama
    model: llama3.2

  primary:
    provider: anthropic


  embedder:
    container:
      build:
        context: ./embedder
        dockerfile: Dockerfile
      port: 8000
      healthcheck:
        path: /health

knowledge:
  docs:
    provider: qdrant
    persistent: true

  cache:
    provider: redis

tools:
  github:
    provider: github

integrations:
  my-service:
    credentials:
      - suffix: API_KEY
        description: API key for my-service
      - suffix: SECRET
        description: Shared secret for HMAC signing
        optional: true

ingestion:
  docs_sync:
    container:
      image: my-docs-sync:latest
      environment:
        SOURCE_REPO: company/engineering-docs
        TARGET_COLLECTION: docs
    trigger:
      type: schedule

  initial_load:
    container:
      image: my-bootstrap-worker:latest
    trigger:
      type: startup

dev:
  interfaces: [slack, web]
  schedules:
    docs_sync: "0 */6 * * *"
  command: bun --watch run start
```
