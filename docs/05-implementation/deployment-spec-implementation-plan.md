# Deployment Spec Implementation Plan

Implementation plan to make `astro-spec.md` and `astro-deployment-spec.md` a reality. Covers every layer from CLI to k8s manifests.

---

## Current State

| Area | Status |
|------|--------|
| Spec parsing (`packages/astro-spec`) | Complete — types, parser, provider registry |
| CLI: `ast build/publish/dev/create/explain` | Complete |
| Server: Agent registry, validation, translation, k8s applier | Working (against old `DeployRequest` flow) |
| Server: Deploy/undeploy/list/logs endpoints | Working |
| Client dashboard: Agent list, deploy form, logs | Basic UI exists |
| Account scoping | Complete — agents keyed by (account_id, name) |
| Build-ID semantics | Complete — `build_id` replaces `version`; semver assigned separately via publish |

### Current deploy flow (what exists today)

```
CLI: ast publish → POST /api/v1/agents/:account/:name/register (spec stored with build_id)
User: POST /api/v1/deploy with DeployRequest { account, name, source_account, user_credentials, interfaces, schedules }
Server: resolves latest build_id → Validator → Translator → K8s manifests → Applier → cluster
```

No intermediate deployment spec artifact. Config is consumed and discarded. No inspectable record of what was deployed.

### Target deploy flow (from deployment-spec.md)

```
CLI: ast publish → spec stored in registry
Server: GET /api/v1/agents/:account/:name/deployment-template → deployment spec template
User: fills in template (credentials, schedules, interfaces, resource overrides)
Server: POST /api/v1/deploy with filled deployment spec → validate → resolve → translate → apply
Server: stores resolved deployment spec (credentials stripped) for audit/redeploy/diff
```

---

## Phase 1 — Deployment Spec Schema

**Package:** `packages/astro-spec`

### 1.1 Define `AstroDeploymentSpec` Go struct

The `deployment/v1` schema. Top-level fields:

- `Spec` (string) — `"deployment/v1"`
- `Source` — `{ Account, Name, Build, Registry }`
- `Target` — `{ Runtime, Namespace }`
- `Agent` — `{ Image, Port, Replicas, Resources, Environment, Healthcheck, Update, Expose }`
- `Models` — `map[string]DeploymentModel` — `{ Image, Port, Replicas, Resources, GPU, Environment, Healthcheck, Update }`
- `Knowledge` — `map[string]DeploymentKnowledge` — `{ Image, Port, Replicas, Resources, Persistent, Storage, Environment, Healthcheck, Update }`
- `Tools` — `map[string]DeploymentTool` — `{ Image, Port, Replicas, Resources, Environment, Healthcheck, Update }`
- `Ingestion` — `map[string]DeploymentIngestion` — `{ Image, Port, Resources, Trigger, Environment, Healthcheck }`
- `Interfaces` — `{ Adapters, Image, Port, Resources, Environment, Healthcheck, Expose }`
- `Credentials` — `map[string]Credential` — `{ Value, Description, Optional }`
- `Observability` — `{ Enabled, Provider }`
- `Editable` — `[]string` (template-only, stripped during resolution)

Sub-types:

- `Resources` — `{ CPU, Memory, CPULimit, MemoryLimit }`
- `GPUConfig` — `{ VRAM, Runtime, Count }`
- `StorageConfig` — `{ Size, Class, AccessMode }`
- `Healthcheck` — reuse from `AstroSpec`
- `UpdateStrategy` — `{ Strategy, MaxUnavailable, MaxSurge }`
- `ExposeConfig` — `{ Enabled, Domain, Port }`

**File:** `packages/astro-spec/deployment_spec.go`

### 1.2 Deployment spec parser/serializer

- `ParseDeploymentSpec([]byte) (*AstroDeploymentSpec, error)` — YAML/JSON
- `SerializeDeploymentSpec(*AstroDeploymentSpec) ([]byte, error)` — YAML output
- Validation: required fields, type constraints

**File:** `packages/astro-spec/deployment_parser.go`

### 1.3 Reference types and parser

Define `${}` reference syntax used in `agent.environment`:

- `${models.<name>.host}`, `${models.<name>.port}`, `${models.<name>.url}`
- `${knowledge.<name>.host}`, `${knowledge.<name>.port}`, `${knowledge.<name>.url}`
- `${tools.<name>.host}`, `${tools.<name>.port}`, `${tools.<name>.url}`
- `${credentials.<KEY>}`
- `${source.name}`, `${source.build}`

Parser extracts references from string values for validation and resolution.

**File:** `packages/astro-spec/references.go`

---

## Phase 2 — Template Generation

**Package:** `apps/astro-server/internal/deployment`

### 2.1 Template generator

`GenerateDeploymentTemplate(spec *AstroSpec, account, registryURL string) (*AstroDeploymentSpec, error)`

Steps:
1. Set `source.account`, `source.name`, `source.build`, `source.registry` from registered spec and account
2. Set `target.runtime: kubernetes`, `target.namespace: ""` (placeholder)
3. Resolve agent image (already resolved at publish time — no build blocks)
4. For each model: if provider mode, resolve via `GetModelProvider()` to image/port; if container mode, copy image/port. Apply resource defaults (GPU-tier for models with GPU, standard otherwise)
5. Same for knowledge (resolve via `GetProvider()`) and tools
6. Populate `agent.environment` with `${}` references using conventional env var names:
   - Models: `MODEL_{UPPER(name)}_HOST/PORT/URL`
   - Knowledge with provider: provider-specific prefix (`QDRANT_*`, `REDIS_*`, `POSTGRES_*`)
   - Knowledge without provider: `KNOWLEDGE_{UPPER(name)}_HOST/PORT`
   - Tools: `TOOL_{UPPER(name)}_HOST/PORT/URL`
   - Credentials: each key maps to itself
   - Platform: `ASTRO_AGENT_NAME`, `ASTRO_AGENT_BUILD`
7. Extract credentials from integrations via existing `GetRequiredCredentials()`. Emit entries with empty values, descriptions, optional flags
8. For schedule-type ingestions: emit `trigger.schedule: ""` placeholder
9. Emit `interfaces` block with `adapters: []`, platform messaging image, default port 9090, resource defaults
10. Apply defaults: `replicas: 1`, `observability.enabled: true`, `agent.expose.enabled: false`
11. Emit `editable` list

**File:** `apps/astro-server/internal/deployment/template.go`

### 2.2 Template generation endpoint

```
GET /api/v1/agents/:account/:name/deployment-template
GET /api/v1/agents/:account/:name/deployment-template?build=<build_id>   (pin to specific build)
```

Handler: fetch latest build (or specified build) from agent index, call `GenerateDeploymentTemplate()`, return YAML.

Replaces the current `GET /api/v1/agents/:account/:name/config` endpoint (which only returns credentials).

**Files:**
- `apps/astro-server/handlers/deploy.go` — new handler
- `apps/astro-server/main.go` — register route

---

## Phase 3 — Validation & Resolution

**Package:** `apps/astro-server/internal/deployment`

### 3.1 Editable-field enforcement

When a filled deployment spec is submitted:
1. Re-generate the template for the same agent build
2. Diff submitted spec against template
3. Any field changed that is NOT in the `editable` list is rejected

Prevents tampering with server-resolved images, ports, source metadata.

### 3.2 Reference validation

Every `${}` reference in `agent.environment` must resolve to:
- A declared model (`models.<name>` exists in spec)
- A declared knowledge store (`knowledge.<name>` exists)
- A declared tool (`tools.<name>` exists)
- A declared credential (`credentials.<KEY>` exists)
- A source attribute (`source.name` or `source.build`)

Invalid references are rejected with descriptive errors.

### 3.3 Resolution

Produce a fully resolved deployment spec:
1. Validate editable enforcement (3.1)
2. Validate references (3.2)
3. Check required credentials non-empty
4. Validate cron expressions for schedule-type ingestions
5. Validate interface adapter names
6. Re-derive interface credentials (e.g. `slack` adapter → require `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`)
7. Apply defaults for any omitted optional fields
8. Strip `editable` field (template-only)
9. Return resolved spec

**File:** `apps/astro-server/internal/deployment/resolver.go`

---

## Phase 4 — Translator Refactor

**Package:** `apps/astro-server/internal/deployment`

### 4.1 New translator signature

Current:
```go
func NewTranslator(agentName, buildID, k8sNamespace, registryURL string,
    userCredentials map[string]string, interfaces []string,
    schedules map[string]string) *Translator
```

New:
```go
func NewTranslator(deploySpec *AstroDeploymentSpec) *Translator
```

All information in one struct. The translator reads image, port, replicas, resources, environment, healthcheck, GPU, update strategy directly from the deployment spec. No `ResolvedContainer()` calls, no provider lookups.

### 4.2 Reference resolution in translator

Resolve `${}` references in `agent.environment` to actual values:
- `${models.llm.host}` → k8s service DNS (e.g. `my-agent-llm-svc.ns.svc.cluster.local`)
- `${models.llm.port}` → `"8000"`
- `${models.llm.url}` → `http://my-agent-llm-svc:8000`
- `${credentials.ANTHROPIC_API_KEY}` → Secret key ref
- `${source.name}` → literal agent name

Write resolved values into ConfigMap (for non-secret values) and Secret refs (for credentials).

### 4.3 Replicas support

Read `replicas` from each component in deployment spec. Set on Deployment/StatefulSet `.spec.replicas`. Currently hardcoded to 1.

### 4.4 Resource requests/limits

Read `resources` from deployment spec per component:
- `cpu` → `container.resources.requests.cpu`
- `memory` → `container.resources.requests.memory`
- `cpu_limit` → `container.resources.limits.cpu`
- `memory_limit` → `container.resources.limits.memory`

Currently uses hardcoded defaults.

### 4.5 Update strategy

Read `update` from deployment spec:
- `strategy: rolling` → `Deployment.spec.strategy.type: RollingUpdate` with `maxUnavailable` and `maxSurge`
- `strategy: recreate` → `Deployment.spec.strategy.type: Recreate`

Currently always uses k8s default (rolling).

### 4.6 Storage config for persistent knowledge

Read `storage` from deployment spec for PVC creation:
- `size` → `PVC.spec.resources.requests.storage`
- `class` → `PVC.spec.storageClassName`
- `access_mode` → `PVC.spec.accessModes`

Currently hardcoded to `10Gi` with cluster default storage class.

**Files:**
- `apps/astro-server/internal/deployment/translator.go` — refactor
- `apps/astro-server/internal/deployment/envbuilder.go` — replace with reference resolver
- `apps/astro-server/internal/k8s/deployment.go` — accept resource/replica/update params

---

## Phase 5 — Deploy API & Storage

### 5.1 Update deploy endpoint

```
POST /api/v1/deploy
Content-Type: application/yaml (or application/json)
Body: filled-in deployment spec
```

New pipeline: parse deployment spec → validate editable enforcement → validate references → resolve → translate → apply.

Replace current `DeployRequest` acceptance with deployment spec acceptance.

**File:** `apps/astro-server/handlers/deploy.go`

### 5.2 Deployment spec storage

Store resolved deployment spec (credential values stripped) after successful deployment.

New `deployments` table — supports multiple deployments of same build (different namespaces/configs) and deployment history.

```sql
CREATE TABLE deployments (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    agent_name VARCHAR NOT NULL,
    build_id VARCHAR NOT NULL,
    deployment_spec_json TEXT NOT NULL,  -- resolved, credentials stripped
    status VARCHAR NOT NULL,             -- active, undeployed
    deployed_at TIMESTAMP NOT NULL,
    undeployed_at TIMESTAMP,
    UNIQUE(account_id, agent_name)      -- one active deployment per agent per account
);
```

**Files:**
- `apps/astro-server/internal/agentindex/` — or new `deploymentstore` package
- `apps/astro-server/migrations/` — new migration

### 5.3 Deployment spec retrieval

```
GET /api/v1/deployments/:account/:name/spec
```

Returns stored resolved deployment spec for the active deployment.

### 5.4 Deployment history

```
GET /api/v1/deployments/:account/:name/history
```

Returns list of past deployment specs for diff/rollback.

---

## Phase 6 — Interfaces & Messaging Sidecar

### 6.1 Messaging sidecar deployment

Translator generates a Deployment for the messaging container when `interfaces.adapters` is non-empty:
- Image from `interfaces.image`
- Port from `interfaces.port` (default 9090 gRPC)
- Environment from `interfaces.environment` (adapter flags, Slack tokens via credential refs)
- Resources from `interfaces.resources`

**File:** `apps/astro-server/internal/deployment/translator.go`

### 6.2 gRPC wiring

Inject `GRPC_SERVER_ADDR` into agent container pointing to messaging sidecar service DNS. Platform-managed env var — injected regardless of `agent.environment` content.

### 6.3 Web adapter exposure

When `interfaces.expose.enabled`:
- Create Ingress for the web adapter HTTP port (`interfaces.expose.port`, default 8080)
- Set domain from `interfaces.expose.domain`

### 6.4 Slack adapter credentials

Wire `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` from `${credentials.*}` references in `interfaces.environment` into messaging sidecar container.

---

## Phase 7 — Observability

### 7.1 Collector sidecar

When `observability.enabled`:
- Deploy OTel collector sidecar (Galileo config)
- Inject `OTEL_EXPORTER_OTLP_ENDPOINT` into agent container

Partially stubbed in current applier. Needs to be wired through deployment spec.

---

## Phase 8 — Client Dashboard

### 8.1 Deployment template form

Fetch template from `GET /api/v1/agents/:account/:name/deployment-template`. Render:
- Editable fields as form inputs (credentials, schedules, replicas, resources, interfaces)
- Server-owned fields as read-only context (images, ports, source metadata)
- Submit filled spec to `POST /api/v1/deploy`

### 8.2 Credential input UI

- Display required credentials with descriptions and optional flags
- Secure input fields (password-type)
- Validation feedback

### 8.3 Deployment diff/audit view

- Show stored resolved deployment specs
- Diff between builds or deployment attempts
- Rollback button (redeploy previous spec)

---

## Dependency Graph

```
Phase 1 (types)
  |
  v
Phase 2 (template generation)
  |
  v
Phase 3 (validation/resolution)
  |
  v
Phase 4 (translator refactor) ──+── Phase 5 (API/storage)
                                 |
              +------------------+------------------+
              |                  |                  |
              v                  v                  v
  Phase 6 (interfaces)   Phase 7 (observability)   Phase 8 (dashboard)
```

Phases 6, 7, 8 can proceed in parallel once Phases 4-5 are done. Phases 6 and 7 are independent of each other.

---

## Key Files Summary

| Concern | Package | File(s) |
|---------|---------|---------|
| Deployment spec types | `packages/astro-spec` | `deployment_spec.go` (new) |
| Deployment spec parser | `packages/astro-spec` | `deployment_parser.go` (new) |
| Reference parser | `packages/astro-spec` | `references.go` (new) |
| Template generation | `internal/deployment` | `template.go` (new) |
| Resolution/validation | `internal/deployment` | `resolver.go` (new) |
| Translator refactor | `internal/deployment` | `translator.go` (modify) |
| EnvBuilder → reference resolver | `internal/deployment` | `envbuilder.go` (replace) |
| Deploy handler | `handlers` | `deploy.go` (modify) |
| Deployment storage | `internal/agentindex` or new | migration + store code |
| Routes | `main.go` | add template endpoint |
