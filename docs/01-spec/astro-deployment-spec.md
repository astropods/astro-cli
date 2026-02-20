# Astro Deployment Spec

The Astro Deployment Spec is a declarative document that describes how to deploy a specific version of an agent. It sits between the astro-spec (what the agent is) and infrastructure manifests (how it runs on a cluster). The astro-spec defines topology and components; the deployment spec adds runtime configuration — credentials, interfaces, schedules, replicas, exposure — needed to produce a running deployment.

Today, the deploy API performs an implicit, in-memory transformation: `AstroSpec + DeployRequest → k8s manifests`. The deployment spec makes this intermediate step explicit, inspectable, storable, and diffable.

## Pipeline

```
astroai.yml (author-time, defines agent topology)
    │
    │  publish (build images, push, strip build blocks, register)
    ▼
registered spec (image refs only, no build blocks)
    │
    │  GET /api/v1/agents/:name/:version/deployment-template
    │  (server introspects spec, resolves providers, enumerates credentials)
    ▼
deployment spec template (placeholders + descriptions — user fills runtime config)
    │
    │  POST /api/v1/deploy (server validates, resolves defaults, stores)
    ▼
resolved deployment spec (fully resolved, no placeholders, stored)
    │
    │  translator (deterministic, mechanical, no provider lookups)
    ▼
k8s manifests (or other target runtime)
```

Three phases:

1. **Template generation (server API)** — Server reads registered spec, resolves self-hosted providers to concrete images/ports, enumerates required credentials from cloud providers and integrations, and returns a deployment spec template with placeholder values and descriptions.
2. **Resolution** — User fills in the template. Server validates (credentials present, cron valid, images accessible), applies defaults, produces a fully resolved spec. This resolved spec is the deployment contract: same resolved spec always produces same manifests.
3. **Translation** — Deterministic structural mapping from resolved spec to target infrastructure. No business logic, no provider lookups, no credential discovery.

## Why

**No inspectable artifact.** The translator, env builder, and applier all contribute deployment-time decisions that users cannot see before they hit the cluster. The deployment spec is the single source of truth for what will be deployed.

**Ephemeral deploy config.** The current `DeployRequest` body (credentials, interfaces, schedules) is consumed and discarded. No record of which configuration produced a running stack.

**Tight k8s coupling.** The translator produces k8s-specific manifest structs. The deployment spec is runtime-agnostic — containers, ports, persistence, scheduling — allowing translation to other targets (ECS, Compose) without restructuring.

**No template generation path.** Users who discover an agent in the registry cannot generate a fill-in-the-blanks config. Required credentials and schedule slots are only discoverable at validation time inside the server.

## Schema

```yaml
spec: deployment/v1

source:
  name: string                    # agent name (from astro-spec)
  version: string                 # agent version (from astro-spec meta.version)
  registry: string                # registry where images were pushed

target:
  runtime: string                 # "kubernetes" (extensible: "ecs", "compose")
  namespace: string               # target namespace / isolation boundary

agent:
  image: string                   # resolved image ref
  port: int                       # default 8080
  replicas: int                   # default 1
  resources: Resources            # cpu/memory requests and limits
  environment: map[string]string  # env vars for the agent container — supports ${} references
  healthcheck: Healthcheck        # from spec, overridable
  update: UpdateStrategy          # how to roll out changes
  expose:
    enabled: bool                 # whether to create external access (ingress)
    domain: string                # e.g. "my-agent.astro.example.com"

models:
  <name>:                           # only self-hosted models appear here; cloud providers become credentials only
    image: string                 # concrete image (provider already resolved)
    port: int                     # concrete port (provider already resolved)
    replicas: int                 # default 1
    resources: Resources
    gpu: GPUConfig
    environment: map[string]string # env vars for this model's own container
    healthcheck: Healthcheck
    update: UpdateStrategy
    # exposes: <name>.host, <name>.port, <name>.url

knowledge:
  <name>:
    image: string
    port: int
    replicas: int
    resources: Resources
    persistent: bool              # triggers StatefulSet vs Deployment
    storage: StorageConfig        # PVC configuration, required when persistent
    environment: map[string]string # env vars for this store's own container
    healthcheck: Healthcheck
    update: UpdateStrategy
    # exposes: <name>.host, <name>.port, <name>.url

tools:
  <name>:
    image: string
    port: int
    replicas: int
    resources: Resources
    environment: map[string]string # env vars for this tool's own container
    healthcheck: Healthcheck
    update: UpdateStrategy
    # exposes: <name>.host, <name>.port, <name>.url

ingestion:
  <name>:
    image: string
    port: int                     # for webhook type
    resources: Resources
    trigger:
      type: string                # schedule | startup | webhook | manual
      schedule: string            # cron expression, required when type=schedule
    environment: map[string]string
    healthcheck: Healthcheck

interfaces:
  adapters: [string]                # enabled adapters (e.g. ["slack", "web"])
  image: string                     # messaging sidecar image
  port: int                         # gRPC port (default 9090)
  resources: Resources
  environment: map[string]string    # adapter-specific env vars — supports ${credentials.*} references
  healthcheck: Healthcheck
  expose:
    enabled: bool                   # create external HTTP access (for web adapter)
    port: int                       # HTTP port when exposed (default 8080)
    domain: string

credentials:
  <KEY>:
    value: string                 # secret value (filled by user, stripped before storage)
    description: string           # human hint (from template generation)
    optional: bool                # whether deployment can proceed without it

observability:
  enabled: bool                   # deploy collector sidecar (default true)
  provider: string                # "galileo" (extensible)
  log_stream: string              # Galileo log stream name (default: "{source.name}-{deployment_id}")

# template-only metadata (stripped during resolution)
editable: [string]                  # field paths the user may modify — all other fields are server-owned
```

### Resources

CPU and memory requests/limits. The runtime uses `requests` for scheduling and `limits` for enforcement. All fields optional — the server applies defaults based on component type and GPU config when omitted.

```yaml
resources:
  cpu: string                     # request (e.g. "100m", "2")
  memory: string                  # request (e.g. "256Mi", "8Gi")
  cpu_limit: string               # limit (e.g. "1", "4")
  memory_limit: string            # limit (e.g. "1Gi", "16Gi")
```

Defaults applied by the server when omitted:

| Component                               | CPU request | Memory request | CPU limit | Memory limit |
| --------------------------------------- | ----------- | -------------- | --------- | ------------ |
| Standard (agent, tools, non-GPU models) | 100m        | 256Mi          | 1         | 1Gi          |
| GPU workloads (models with `gpu`)       | 2           | 8Gi            | 4         | 16Gi         |
| Messaging sidecars                      | 100m        | 128Mi          | 500m      | 512Mi        |
| Collector                               | 50m         | 128Mi          | 250m      | 256Mi        |

### GPUConfig

```yaml
gpu:
  vram: string                    # e.g. "24Gi" — scheduling hint for GPU memory
  runtime: string                 # "cuda" (default) or "rocm"
  count: int                      # number of GPUs (default 1)
```

Maps to: node selector (`accelerator: nvidia-gpu` or `amd-gpu`), `nvidia.com/gpu` resource request, and GPU-tier resource defaults.

### StorageConfig

```yaml
storage:
  size: string                    # PVC size (e.g. "10Gi"), default "10Gi"
  class: string                   # storage class name (e.g. "gp3", "io1"), omit for cluster default
  access_mode: string             # "ReadWriteOnce" (default) or "ReadWriteMany"
```

### Healthcheck

Defines both liveness and readiness probes. The runtime creates identical probes from this config.

```yaml
healthcheck:
  test: [string]                  # exec probe command (e.g. ["CMD", "redis-cli", "ping"])
  path: string                    # HTTP path (e.g. "/health"), generates HTTP GET probe
  initial_delay: string           # time before first check (default "10s")
  interval: string                # check frequency (default "10s")
  timeout: string                 # per-check timeout (default "5s")
  retries: int                    # failures before unhealthy (default 3)
```

When neither `test` nor `path` is set, the server generates a provider-appropriate probe during template generation (e.g. `redis-cli ping` for redis, HTTP `/healthz` for qdrant).

### UpdateStrategy

Controls how changes are rolled out. Only applicable to long-running workloads (agent, models, knowledge, tools, webhook ingestion). Ignored for jobs/cronjobs.

```yaml
update:
  strategy: string                # "rolling" (default) or "recreate"
  max_unavailable: string         # rolling only: max pods down during update (default "25%")
  max_surge: string               # rolling only: max extra pods during update (default "25%")
```

`recreate` kills all existing pods before creating new ones — necessary for workloads that cannot tolerate two versions running simultaneously (e.g. a knowledge store with exclusive volume access). `rolling` replaces pods incrementally.

## Key Design Decisions

### Images always resolved, never built

The deployment spec only contains image references. All `build` blocks were resolved to images during publish. The deployment spec references immutable OCI artifacts — it is fully portable.

### Provider resolution at template generation, not translation

Today `model.ResolvedContainer()` and `knowledge.ResolvedContainer()` resolve provider names (e.g. `ollama`, `qdrant`) to images and ports at translation time. In the new design, this happens during template generation. The deployment spec contains `image: qdrant/qdrant:latest, port: 6333` — no provider names. This makes the translator purely structural.

### Editable fields and server-owned fields

The template response includes an `editable` list — field paths the user is allowed to modify. Every field not in this list is server-owned and rejected at validation if changed. This serves two purposes:

1. **Safety.** Server-resolved values (provider images, ports, source metadata, platform container images) cannot be accidentally or intentionally altered. The server is the authority for these values.
2. **UI/CLI guidance.** A dashboard or CLI can render only the editable fields as form inputs and display server-owned fields as read-only context. No guesswork about what's fillable.

The `editable` list uses dot-path notation with wildcards for map entries: `credentials.*.value` means the `value` field of every credential entry is editable, while `credentials.*.description` is not. The list is emitted during template generation and stripped during resolution — the resolved spec has no `editable` field.

### Component references and agent environment wiring

Every model, knowledge store, and tool implicitly exposes three attributes: `.host`, `.port`, `.url`. The `agent.environment` block uses `${}` references to wire these into the agent container's env vars. All wiring is visible in one place — the user controls the env var names and can see exactly how components connect.

**Reference syntax:** `${<section>.<name>.<attribute>}`

Available references:

| Reference                  | Resolves to                                                              |
| -------------------------- | ------------------------------------------------------------------------ |
| `${models.<name>.host}`    | Service DNS for the model                                                |
| `${models.<name>.port}`    | Port number (string)                                                     |
| `${models.<name>.url}`     | `http://<host>:<port>`                                                   |
| `${knowledge.<name>.host}` | Service DNS for the knowledge store                                      |
| `${knowledge.<name>.port}` | Port number (string)                                                     |
| `${knowledge.<name>.url}`  | `<scheme>://<host>:<port>` (scheme from provider: `http`, `redis`, etc.) |
| `${tools.<name>.host}`     | Service DNS for the tool                                                 |
| `${tools.<name>.port}`     | Port number (string)                                                     |
| `${tools.<name>.url}`      | `http://<host>:<port>`                                                   |
| `${credentials.<KEY>}`     | Credential value (resolved from Secret at runtime)                       |
| `${source.name}`           | Agent name                                                               |
| `${source.version}`        | Agent version                                                            |

**Example wiring in `agent.environment`:**

```yaml
agent:
  environment:
    # model connections
    OLLAMA_URL: "${models.local_llm.url}"
    OLLAMA_HOST: "${models.local_llm.host}"

    # knowledge connections
    QDRANT_HOST: "${knowledge.docs.host}"
    QDRANT_PORT: "${knowledge.docs.port}"
    QDRANT_URL: "${knowledge.docs.url}"
    REDIS_URL: "${knowledge.cache.url}"

    # tool connections
    WEBSEARCH_URL: "${tools.websearch.url}"

    # credentials
    ANTHROPIC_API_KEY: "${credentials.ANTHROPIC_API_KEY}"
    GITHUB_TOKEN: "${credentials.GITHUB_TOKEN}"

    # metadata
    AGENT_NAME: "${source.name}"
    AGENT_VERSION: "${source.version}"

    # static values (no reference)
    LOG_LEVEL: "info"
```

The user picks the env var names. The agent code reads `OLLAMA_URL`, `QDRANT_HOST`, etc. — whatever names the user chose. There is no implicit naming convention imposed by the platform.

**How template generation populates `agent.environment`:**

The server knows the conventional env var names from the provider registry and current `EnvBuilder` logic. It emits the `agent.environment` block pre-wired with these conventions:

- Models: `MODEL_{UPPER(name)}_HOST`, `MODEL_{UPPER(name)}_PORT`, `MODEL_{UPPER(name)}_URL`
- Knowledge with provider: uses provider's env prefix (`QDRANT_*`, `REDIS_*`, `POSTGRES_*`)
- Knowledge without provider: `KNOWLEDGE_{UPPER(name)}_HOST`, `KNOWLEDGE_{UPPER(name)}_PORT`
- Tools: `TOOL_{UPPER(name)}_HOST`, `TOOL_{UPPER(name)}_PORT`, `TOOL_{UPPER(name)}_URL`
- Credentials: each credential key maps to itself
- Platform vars: `ASTRO_AGENT_NAME`, `ASTRO_AGENT_VERSION`, `OTEL_EXPORTER_OTLP_ENDPOINT`

The user can rename, remove, or add entries. The template is a starting point, not a constraint.

**Platform-managed env vars:**

Some env vars are injected by the translator regardless of `agent.environment`: `GRPC_SERVER_ADDR` (when interfaces are enabled) and `OTEL_EXPORTER_OTLP_ENDPOINT` (when observability is enabled). These are platform infrastructure concerns, not component wiring. They are still shown in the template for visibility but annotated as platform-managed.

**Why references instead of implicit injection:**
- All agent env vars visible in one block — no hidden injection from scattered components.
- User controls the names — agent code can use whatever env var names it wants.
- Explicit wiring is auditable — you can see exactly how components connect.
- References are stable — changing server-side naming conventions doesn't break existing deployment specs.
- A web dashboard can parse the references to render a wiring diagram.

### Credentials are extracted from the astro-spec and declared in the template

Credential requirements are derived from cloud providers declared in `models`, `knowledge`, and `tools` sections, integrations in the `integrations` section, and the deployment-time `interfaces` list. The template generation API extracts them so the user sees exactly which secrets are needed, why, and whether they're optional.

**Extraction sources:**

1. **Cloud providers (from astro-spec models/knowledge/tools).** The server's provider registry classifies each provider as `self-hosted` (has image/port) or `cloud` (has credential suffixes). When a cloud provider is encountered in any section, the credential env var key is `{UPPER(entry_name)}_{suffix}`.

   Example: a model named `primary` with `provider: anthropic` produces `PRIMARY_API_KEY` because the `anthropic` provider defines suffix `API_KEY`. A model named `anthropic` with `provider: anthropic` produces `ANTHROPIC_API_KEY`. A tool named `github` with `provider: github` produces `GITHUB_TOKEN`.

   The entry's map key determines the env var prefix, not the provider name. So a model named `fallback` with `provider: openai` produces `FALLBACK_API_KEY`, not `OPENAI_API_KEY`.

2. **Integrations (from astro-spec).** Entries in the `integrations` section declare an explicit `credentials` array with suffixes, descriptions, and optional flags. These are passed through directly: `{UPPER(name)}_{suffix}`.

3. **Interfaces (deployment-time).** Messaging interfaces like `slack` require their own credentials (`SLACK_APP_TOKEN`, `SLACK_BOT_TOKEN`). Since adapters are a deployment-time choice (not in the astro-spec), the template generation API cannot know upfront which interface credentials to include. Instead, the template emits `interfaces.adapters: []` with an annotation listing available adapters. When the user fills in `interfaces.adapters: [slack]` and submits to the deploy endpoint, the server's validation step adds the adapter-derived credentials to the required set and checks them.

**How extraction works during template generation:**

The server calls `validator.GetRequiredCredentials(astroSpec, interfaces)`. Since `interfaces` is empty at template generation time (the user hasn't chosen yet), only spec-derived credentials are emitted. The function:

- Iterates over `astroSpec.Models`, `astroSpec.Knowledge`, `astroSpec.Tools` — for each entry with a cloud provider, looks up the provider in `supportedProviders` to get its credential suffixes, descriptions, and optional flags.
- Iterates over `astroSpec.Integrations` — reads the `credentials` array directly.
- Builds the env var key as `{UPPER(name)}_{suffix}`.
- Returns a `[]CredentialInfo` with key, provider, section, description, and optional flag.

Each `CredentialInfo` maps to one entry in the deployment spec template:

```yaml
credentials:
  PRIMARY_API_KEY:
    value: ""                   # user fills this in
    description: "Anthropic API key for Claude models"
    optional: false
  GITHUB_TOKEN:
    value: ""
    description: "GitHub token for API access"
    optional: false
  MY_SERVICE_API_KEY:           # from integrations
    value: ""
    description: "API key for my-service"
    optional: false
  MY_SERVICE_SECRET:            # optional integration credential
    value: ""
    description: "Shared secret for HMAC signing"
    optional: true
```

**At deploy time:**

When the filled-in deployment spec is POSTed, the server re-runs credential validation. This time `interfaces` is populated from the spec, so interface-derived credentials (e.g. `SLACK_APP_TOKEN`, `SLACK_BOT_TOKEN` for `interfaces: [slack]`) are added to the required set. Validation fails if any required credential has an empty `value`.

**After deployment:**

Credential values are stripped from the stored resolved spec — only key names, descriptions, and optional flags are retained. The actual values are written to a k8s Secret by the translator and never persisted in the deployment record.

### Replicas are a deployment concern

The astro-spec has no concept of replicas. The deployment spec adds `replicas` per component, defaulting to 1.

### Target block enables multi-runtime

`target.runtime` allows the translation layer to be swapped. The deployment spec is runtime-agnostic.

## Relationship to Existing Specs

| Concern             | AstroSpec                                                                           | DeploymentSpec                                               | K8s Manifest                           |
| ------------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------- |
| Build instructions  | `agent.build`                                                                       | absent                                                       | absent                                 |
| Provider name       | `models.x.provider: ollama` (self-hosted) or `models.x.provider: anthropic` (cloud) | absent (resolved to image or credential)                     | absent                                 |
| Model image         | resolved at runtime via `ResolvedContainer()`                                       | `models.x.image: ollama/ollama:latest`                       | `Deployment.containers[0].image`       |
| Connection env vars | implicit (EnvBuilder derives from names)                                            | `agent.environment: {OLLAMA_URL: "${models.local_llm.url}"}` | `ConfigMap.data.OLLAMA_URL`            |
| Credentials         | `models.x.provider: anthropic` (cloud) or `integrations.x.credentials`              | `credentials.X_API_KEY.value: sk-...`                        | `Secret.data.X_API_KEY`                |
| Schedules           | `ingestion.x.trigger.type: schedule`                                                | `ingestion.x.trigger.schedule: "0 * * * *"`                  | `CronJob.spec.schedule`                |
| Interfaces          | absent (in `dev` only)                                                              | `interfaces: {adapters: [slack], image: ..., port: 9090}`    | messaging sidecar Deployment + Service |
| Replicas            | absent                                                                              | `agent.replicas: 2`                                          | `Deployment.spec.replicas: 2`          |
| Resources           | absent (hardcoded defaults)                                                         | `agent.resources: {cpu: 100m, ...}`                          | `container.resources.requests/limits`  |
| Update strategy     | absent (k8s default)                                                                | `agent.update: {strategy: rolling}`                          | `Deployment.spec.strategy`             |
| Namespace           | absent                                                                              | `target.namespace: user-123`                                 | all resources in namespace             |
| Persistence         | `knowledge.x.persistent: true`                                                      | `knowledge.x.persistent: true, storage: ...`                 | StatefulSet + PVC                      |
| Expose/Ingress      | absent                                                                              | `agent.expose.domain: ...`                                   | Ingress resource                       |
| Observability       | absent                                                                              | `observability.enabled: true`                                | collector sidecar Deployment           |

## Template Generation API

### `GET /api/v1/agents/:name/:version/deployment-template`

Reads the registered spec from the agent index and returns a deployment spec template. Provider registries, credential resolution logic, and the spec index all live on the server.

**Response:** deployment spec YAML with placeholder values and descriptions for all fields the user must fill in.

**Steps:**

1. Fetch registered astro-spec from agent index (images resolved, no build blocks).
2. For each model: if self-hosted provider, call `GetModelProvider()` to resolve image and default port — emit a model entry in the deployment spec. If cloud provider, skip container resolution — only extract credentials (step 5). If container mode, copy image/port directly.
3. Same for knowledge (`GetProvider()` for self-hosted providers, credential extraction for cloud providers) and tools.
4. Populate `agent.environment` with `${}` references wiring all self-hosted components, credentials, and platform vars using conventional env var names (see component references section).
5. Extract credentials from cloud providers across all sections (models, knowledge, tools) and from integrations (see Credentials section above for the full extraction logic). Emit `credentials` entries with empty values, descriptions, and optional flags.
6. For each ingestion with `trigger.type: schedule`: emit trigger with `schedule: ""` placeholder.
7. Emit `interfaces` block with `adapters: []`, the platform messaging sidecar image, default port (9090), messaging resource defaults, and an `available_adapters` annotation listing available adapters.
8. Apply defaults: `replicas: 1`, `observability.enabled: true`, `agent.expose.enabled: false`, `target.runtime: kubernetes`.
9. Set `source.name`, `source.version`, `source.registry` from the registered spec metadata.
10. Emit `editable` list enumerating all user-modifiable field paths (see Editable fields section).

## Resolution and Validation

Server validates the filled-in deployment spec:

- **Editable enforcement.** Re-generate the template for the same agent version and diff against the submitted spec. Any field changed that is not in the `editable` list is rejected. This prevents tampering with server-resolved images, ports, and source metadata.
- All `${}` references in `agent.environment` resolve to a declared component, credential, or source attribute. A reference like `${models.foo.url}` is invalid if no model named `foo` exists.
- All required `credentials[*].value` non-empty.
- `ingestion[*].trigger.schedule` valid cron when `type: schedule`.
- `target.namespace` valid for the runtime.
- No duplicate ports within the same deployment scope.

Produces a **resolved deployment spec** — input with defaults applied and namespace sanitized. Stored as the deployment record (credential values stripped).

## Translation

The translator consumes only the resolved `AstroDeploymentSpec`. No other inputs.

Current translator signature:
```go
func NewTranslator(agentName, version, k8sNamespace, registryURL string,
    userCredentials map[string]string, interfaces []string,
    schedules map[string]string) *Translator
```

New translator signature:
```go
func NewTranslator(deploySpec *AstroDeploymentSpec) *Translator
```

All information in one struct. Translation is mechanical:

- For each component: read image, port, replicas, resources, environment, healthcheck, gpu, update strategy directly. No `ResolvedContainer()` calls.
- Resolve `${}` references in `agent.environment`: compute service DNS from component name + `target.namespace`, substitute `${models.x.host}` → actual DNS, `${models.x.port}` → port string, `${models.x.url}` → full URL. Write resolved values into the agent container's ConfigMap.
- Resolve `${credentials.*}` references: create a k8s Secret from credential values, map each reference to a `secretKeyRef` env var.
- Interfaces: read image, port, resources, adapters, and expose config directly. Deploy messaging sidecar container.
- Ingestion: read trigger type + schedule directly (no separate schedules map).

## API Surface

### Template generation

```
GET /api/v1/agents/:name/:version/deployment-template
```

Returns a deployment spec YAML template with placeholders. No auth-derived fields filled in (namespace left empty — filled at deploy time from the authenticated user).

### Deploy

```
POST /api/v1/deploy
Content-Type: application/yaml (or application/json)
Body: filled-in deployment spec
```

The request body is the filled-in deployment spec YAML/JSON. The server parses, validates editable enforcement, resolves, stores, translates, and applies.

## Storage

The resolved deployment spec (credential values stripped) is stored alongside the agent version in the agent index. This provides:

- **Audit trail** — what configuration produced the running stack.
- **Redeploy** — POST the stored spec to recreate the same stack.
- **Diff** — compare deployment specs across versions or environments.
- **Rollback** — redeploy a previous spec.

## Complete Example

Given this astro-spec:

```yaml
spec: astro/v1
name: engineering-assistant

meta:
  version: 2.0.0
  description: Engineering knowledge assistant
  tags: [engineering, support]

agent:
  image: registry.astro.dev/acme/engineering-assistant:2.0.0

models:
  local_llm:
    provider: ollama

  anthropic:
    provider: anthropic

knowledge:
  docs:
    provider: qdrant
    persistent: true

tools:
  github:
    provider: github

ingestion:
  docs_sync:
    container:
      image: registry.astro.dev/acme/engineering-assistant-ingestion-docs-sync:2.0.0
      environment:
        SOURCE_REPO: company/engineering-docs
        TARGET_COLLECTION: docs
    trigger:
      type: schedule
```

`GET /api/v1/agents/engineering-assistant/2.0.0/deployment-template` returns:

```yaml
spec: deployment/v1

source:
  name: engineering-assistant
  version: 2.0.0
  registry: registry.astro.dev/acme

target:
  runtime: kubernetes
  namespace: ""                           # filled by server from authenticated user

agent:
  image: registry.astro.dev/acme/engineering-assistant:2.0.0
  port: 8080
  replicas: 1
  resources:
    cpu: "100m"
    memory: "256Mi"
    cpu_limit: "1"
    memory_limit: "1Gi"
  environment:
    # model connections — wired via references
    MODEL_LOCAL_LLM_HOST: "${models.local_llm.host}"
    MODEL_LOCAL_LLM_PORT: "${models.local_llm.port}"
    MODEL_LOCAL_LLM_URL: "${models.local_llm.url}"

    # knowledge connections — conventional qdrant env vars
    QDRANT_HOST: "${knowledge.docs.host}"
    QDRANT_PORT: "${knowledge.docs.port}"
    QDRANT_URL: "${knowledge.docs.url}"

    # credentials
    ANTHROPIC_API_KEY: "${credentials.ANTHROPIC_API_KEY}"
    GITHUB_TOKEN: "${credentials.GITHUB_TOKEN}"

    # platform metadata
    ASTRO_AGENT_NAME: "${source.name}"
    ASTRO_AGENT_VERSION: "${source.version}"
  update:
    strategy: rolling
    max_unavailable: "25%"
    max_surge: "25%"
  expose:
    enabled: false
    domain: ""

models:
  local_llm:
    image: ollama/ollama:latest           # resolved from provider: ollama
    port: 11434
    replicas: 1
    resources:
      cpu: "2"
      memory: "8Gi"
      cpu_limit: "4"
      memory_limit: "16Gi"
    gpu:
      vram: "24Gi"
      runtime: cuda
      count: 1
    update:
      strategy: recreate

knowledge:
  docs:
    image: qdrant/qdrant:latest           # resolved from provider: qdrant
    port: 6333
    replicas: 1
    resources:
      cpu: "100m"
      memory: "256Mi"
      cpu_limit: "1"
      memory_limit: "1Gi"
    persistent: true
    storage:
      size: "10Gi"
      class: ""                           # omit for cluster default
      access_mode: ReadWriteOnce
    healthcheck:                          # resolved from qdrant provider registry
      path: /healthz
      initial_delay: "10s"
      interval: "10s"
      timeout: "5s"
      retries: 3
    update:
      strategy: recreate

ingestion:
  docs_sync:
    image: registry.astro.dev/acme/engineering-assistant-ingestion-docs-sync:2.0.0
    resources:
      cpu: "100m"
      memory: "256Mi"
      cpu_limit: "1"
      memory_limit: "1Gi"
    trigger:
      type: schedule
      schedule: ""                        # REQUIRED: cron expression (e.g. "0 */6 * * *")
    environment:
      SOURCE_REPO: company/engineering-docs
      TARGET_COLLECTION: docs

interfaces:
  adapters: [slack, web]                    # available: ["slack", "web"]
  image: astromodeai/astro-messaging:latest
  port: 9090                                # gRPC — always enabled
  resources:
    cpu: "100m"
    memory: "128Mi"
    cpu_limit: "500m"
    memory_limit: "512Mi"
  environment:
    GRPC_LISTEN_ADDR: ":9090"
    STORAGE_TYPE: memory
    DEPLOYMENT_MODE: all
    SLACK_ENABLED: "true"
    SLACK_SOCKET_MODE: "true"
    SLACK_BOT_TOKEN: "${credentials.SLACK_BOT_TOKEN}"
    SLACK_APP_TOKEN: "${credentials.SLACK_APP_TOKEN}"
    WEB_ENABLED: "true"
    WEB_LISTEN_ADDR: ":8080"
  expose:
    enabled: true
    port: 8080
    domain: ""

credentials:
  ANTHROPIC_API_KEY:
    value: ""                             # REQUIRED: Anthropic API key for Claude models (from models.anthropic)
    description: Anthropic API key for Claude models
    optional: false
  GITHUB_TOKEN:
    value: ""                             # REQUIRED: GitHub token for API access (from tools.github)
    description: GitHub token for API access
    optional: false
  SLACK_BOT_TOKEN:
    value: ""                             # REQUIRED: Slack bot token for messaging (from interfaces: [slack])
    description: Slack bot token for messaging
    optional: false
  SLACK_APP_TOKEN:
    value: ""                             # REQUIRED: Slack app token for socket mode (from interfaces: [slack])
    description: Slack app token for socket mode
    optional: false

observability:
  enabled: true
  provider: galileo

editable:
  - target.namespace
  - agent.replicas
  - agent.resources
  - agent.environment
  - agent.healthcheck
  - agent.update
  - agent.expose
  - models.*.replicas
  - models.*.resources
  - models.*.gpu
  - models.*.environment
  - models.*.healthcheck
  - models.*.update
  - knowledge.*.replicas
  - knowledge.*.resources
  - knowledge.*.storage
  - knowledge.*.environment
  - knowledge.*.healthcheck
  - knowledge.*.update
  - ingestion.*.resources
  - ingestion.*.trigger.schedule
  - ingestion.*.environment
  - interfaces.adapters
  - interfaces.resources
  - interfaces.expose
  - credentials.*.value
  - observability.enabled
```
