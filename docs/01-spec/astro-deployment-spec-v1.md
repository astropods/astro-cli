# Astro Deployment Spec (deployment-template/v1, deployment/v1)

**Version:** 1.0
**Date:** 2026-02-18
**Status:** Draft

## Abstract

The Astro Deployment Spec defines two related declarative YAML formats for describing how to deploy a specific version of an agent. A **deployment template** (`deployment-template/v1`) is generated from an AstroAI Spec and contains the full variable schema — including UI rendering hints and editable field metadata — for the user to fill in. A **fulfilled deployment spec** (`deployment/v1`) is what the user submits: it contains only the runtime fields needed to produce infrastructure manifests, with UI metadata stripped. Both sit between the AstroAI Spec (agent topology) and infrastructure manifests (runtime resources). A conforming fulfilled spec deterministically produces identical infrastructure manifests.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Introduction

The deployment spec occupies the middle stage of a three-stage pipeline:

```
AstroAI Spec → Deployment Spec → Infrastructure Manifests
```

The AstroAI Spec declares agent topology using provider bindings — components reference named providers that the platform classifies as self-hosted or cloud. The deployment spec eliminates this indirection: self-hosted providers become concrete images and ports, cloud and custom providers become variable entries, and container-mode entries carry through directly. The result is a document with no provider names — only images, ports, and variables.

Processing occurs in three phases:

1. **Template generation** — The server reads a registered AstroAI Spec and resolves providers. Self-hosted providers are looked up in the platform's provider registry to produce an image and port. Cloud and custom providers are converted to variable entries with placeholder values. The output is a `deployment-template/v1` document the user can fill in. It includes the full variable schema (UI hints, defaults, targets) and the `editable` field list.
2. **Fulfillment** — A template is not deployable on its own. It contains empty variable values, missing schedules, and unselected adapters — decisions that only the deploying user can make. The user fills in these fields and submits the spec. The server validates (variables present, cron expressions valid, editable constraints enforced), strips template-only fields, and produces a `deployment/v1` document. The fulfilled spec is the deployment contract: the same fulfilled spec MUST always produce the same infrastructure manifests.
3. **Translation** — Deterministic structural mapping from the `deployment/v1` spec to target infrastructure manifests. The translator consumes only the fulfilled deployment spec — no provider lookups, no variable discovery. Because the spec describes workloads in runtime-agnostic terms (containers, ports, replicas, persistence, scheduling), different translators can target different platforms. A Kubernetes translator produces Deployments, Services, and CronJobs; an ECS translator could produce Task Definitions and Services from the same fulfilled spec. The `target.runtime` field selects which translator is used.

### Scope

This spec covers the schema and validation rules for deployment spec documents. It does NOT cover template generation algorithms, translation logic, or infrastructure manifest formats — those are implementation concerns.

---

## 2. Top-Level Structure

A conforming document MUST contain the following top-level fields:

| Field           | Type                          | Required     | Description                                                   |
| --------------- | ----------------------------- | ------------ | ------------------------------------------------------------- |
| `spec`          | string                        | **REQUIRED** | Spec version identifier. MUST be `deployment-template/v1` or `deployment/v1`. |
| `source`        | object                        | **REQUIRED** | Agent source metadata (Section 3).                            |
| `target`        | object                        | **REQUIRED** | Deployment target (Section 4).                                |
| `agent`         | object                        | **REQUIRED** | Agent container configuration (Section 5).                    |
| `models`        | map\<string, ModelEntry\>     | OPTIONAL     | Self-hosted model entries (Section 6.1).                      |
| `knowledge`     | map\<string, KnowledgeEntry\> | OPTIONAL     | Knowledge store entries (Section 6.2).                        |
| `tools`         | map\<string, ToolEntry\>      | OPTIONAL     | Tool service entries (Section 6.3).                           |
| `ingestion`     | map\<string, IngestionEntry\> | OPTIONAL     | Ingestion pipeline entries (Section 7).                       |
| `interfaces`    | object                        | OPTIONAL     | Messaging sidecar configuration (Section 8). Only present when the agent supports messaging. |
| `variables`     | map\<string, Variable\>       | OPTIONAL     | Deployment variables (Section 9).                             |
| `observability` | object                        | OPTIONAL     | Observability configuration (Section 10).                     |
| `editable`      | string[]                      | OPTIONAL     | `deployment-template/v1` only. Lists editable field paths (Section 12). MUST NOT be present in `deployment/v1`. |

Cloud providers from the AstroAI Spec do NOT appear in `models`, `knowledge`, or `tools`. They are represented solely as `variables` entries.

---

## 3. Source

The `source` object identifies the agent and build being deployed.

| Field      | Type   | Required     | Description                                            |
| ---------- | ------ | ------------ | ------------------------------------------------------ |
| `name`     | string | **REQUIRED** | Agent name from the AstroAI Spec.                      |
| `build`    | string | **REQUIRED** | Build identifier.                                                  |
| `registry` | string | **REQUIRED** | Registry where agent images were pushed.               |

---

## 4. Target

The `target` object specifies where the deployment runs.

| Field       | Type   | Required     | Description                                                              |
| ----------- | ------ | ------------ | ------------------------------------------------------------------------ |
| `runtime`   | string | **REQUIRED** | Target runtime. MUST be `kubernetes`. Implementations MAY add additional runtimes. |
| `namespace` | string | **REQUIRED** | Target namespace. MUST be unique within the cluster.                     |

---

## 5. Agent

The `agent` object configures the primary agent container.

| Field           | Type                          | Required     | Description                                                       |
| --------------- | ----------------------------- | ------------ | ----------------------------------------------------------------- |
| `image`         | string                        | **REQUIRED** | Resolved container image reference.                               |
| `endpoints`     | map\<string, Endpoint\>       | **REQUIRED** | Named network endpoints the agent serves (Section 13.6).          |
| `distributed`   | boolean                       | OPTIONAL     | Whether the agent supports multiple replicas. Default: `false`.   |
| `replicas`      | integer                       | OPTIONAL     | Number of replicas. Default: `1`.                                 |
| `resources`     | Resources                     | OPTIONAL     | CPU and memory configuration (Section 13.1).                      |
| `environment`   | map\<string, string\>         | OPTIONAL     | Environment variables. Supports `${}` references (Section 11).    |
| `healthcheck`   | Healthcheck                   | OPTIONAL     | Health check configuration (Section 13.4).                        |
| `update`        | UpdateStrategy                | OPTIONAL     | Rollout strategy (Section 13.5).                                  |

When the agent declares `interfaces.frontend: true` in the AstroAI Spec, the template generator sets the agent's HTTP endpoint to port 80 with `expose.enabled: true`. This creates an ingress directly to the agent container, bypassing the messaging sidecar.

---

## 6. Component Sections: Models, Knowledge, Tools

Components represent self-hosted services deployed alongside the agent. All provider resolution has already occurred during template generation — entries contain concrete images and ports, not provider names.

### 6.1 Models

Each entry in the `models` map configures a self-hosted model container.

| Field         | Type                    | Required     | Description                                                    |
| ------------- | ----------------------- | ------------ | -------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                            |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 13.6).                        |
| `model`       | string                  | OPTIONAL     | Provider-specific model identifier (e.g. `llama3.2`). Carried from AstroAI Spec. |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                              |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 13.1).                   |
| `gpu`         | GPUConfig               | OPTIONAL     | GPU resource requirements (Section 13.2).                      |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the model container.                 |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 13.4).                     |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 13.5).                               |

### 6.2 Knowledge

Each entry in the `knowledge` map configures a knowledge store container.

| Field         | Type                    | Required     | Description                                                    |
| ------------- | ----------------------- | ------------ | -------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                            |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 13.6).                        |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                              |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 13.1).                   |
| `persistent`  | boolean                 | OPTIONAL     | Whether to use persistent storage (StatefulSet). Default: `false`. |
| `storage`     | StorageConfig           | OPTIONAL     | PVC configuration (Section 13.3). REQUIRED when `persistent` is `true`. |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the knowledge store container.       |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 13.4).                     |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 13.5).                               |

### 6.3 Tools

Each entry in the `tools` map configures a tool service container.

| Field         | Type                    | Required     | Description                                                    |
| ------------- | ----------------------- | ------------ | -------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                            |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 13.6).                        |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                              |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 13.1).                   |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the tool container.                  |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 13.4).                     |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 13.5).                               |

---

## 7. Ingestion

Each entry in the `ingestion` map configures a data ingestion pipeline. Ingestion entries are flattened from the AstroAI Spec's `container` object: `image` replaces `container.image`, and `endpoints` replaces the single `container.port`.

| Field         | Type                  | Required     | Description                                                    |
| ------------- | --------------------- | ------------ | -------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Container image for the ingestion pipeline.                    |
| `endpoints`   | map\<string, Endpoint\> | OPTIONAL     | Named network endpoints (Section 13.6). Applicable when `trigger.type` is `webhook`. |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 13.1).                   |
| `trigger`     | IngestionTrigger        | **REQUIRED** | Trigger configuration (Section 7.1).                           |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the ingestion container.             |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 13.4).                     |

### 7.1 Ingestion Trigger

| Field      | Type   | Required     | Description                                                                 |
| ---------- | ------ | ------------ | --------------------------------------------------------------------------- |
| `type`     | string | **REQUIRED** | MUST be one of: `schedule`, `startup`, `manual`, `webhook`.                 |
| `schedule` | string | Conditional  | Cron expression. REQUIRED when `type` is `schedule`. MUST NOT be present otherwise. |

---

## 8. Interfaces

The `interfaces` object configures messaging adapters (e.g. Slack, web) deployed as a sidecar. This block is only present when the agent supports messaging (`interfaces.messaging: true` or `interfaces` omitted in the AstroAI Spec). When the agent disables messaging, this block MUST be absent and no sidecar is deployed.

| Field         | Type                    | Required     | Description                                                    |
| ------------- | ----------------------- | ------------ | -------------------------------------------------------------- |
| `adapters`    | string[]                | **REQUIRED** | Enabled adapter names (e.g. `["slack", "web"]`).               |
| `image`       | string                  | **REQUIRED** | Messaging sidecar container image.                             |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 13.6).                        |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 13.1).                   |
| `environment` | map\<string, string\>   | OPTIONAL     | Adapter-specific environment variables. Supports `${}` references. |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 13.4).                     |

---

## 9. Variables

Each entry in the `variables` map declares a deployment variable. Variables are derived from three sources during template generation:

- **Providers** — each provider referenced by a component produces one variable entry per declared variable. The provider's definition determines the variable name, and all `Input` fields carry through transparently.
- **Interfaces** — messaging adapters (e.g. `slack`) produce variable entries for adapter-specific configuration. All `Input` fields carry through transparently.
- **Inputs** — `inputs` entries from the AstroAI Spec (top-level, `agent.inputs`, and per-component) produce variable entries. All `Input` fields carry through transparently. These variables are referenced via `${variables.<name>}` in `environment` fields.

| Field      | Type     | Required     | Description                                                                                   |
| ---------- | -------- | ------------ | --------------------------------------------------------------------------------------------- |
| `targets`  | string[] | **REQUIRED** | Containers this variable is injected into. Each value MUST be one of: `agent`, `ingestion` (all pipelines), `ingestion.<name>` (specific pipeline), or `interface.<adapter>` (e.g. `interface.slack`). User-editable. |
| `secret`   | boolean  | OPTIONAL     | Whether the value is sensitive. Default: `false`. Secret values are stored in a Secret and never logged. Non-secret values are injected as plain env vars. |
| `optional` | boolean  | OPTIONAL     | Whether deployment MAY proceed without this variable. Default: `false`.                       |

The user-supplied value is represented differently by each spec type: `deployment-template/v1` uses `default` (the pre-filled suggestion shown in the UI); `deployment/v1` uses `value` (the actual supplied value, stripped before storage when `secret: true`). See Section 12.1 for `default`.

---

## 10. Observability

The `observability` object configures monitoring and telemetry.

| Field        | Type    | Required | Description                                                              |
| ------------ | ------- | -------- | ------------------------------------------------------------------------ |
| `enabled`    | boolean | OPTIONAL | Whether to deploy a collector sidecar. Default: `true`.                  |
| `provider`   | string  | OPTIONAL | Observability provider name (e.g. `galileo`).                            |
| `log_stream` | string  | OPTIONAL | Provider-specific log stream name. Default: `{source.name}-{deployment_id}`. |

---

## 11. Component References

Environment variables in `agent.environment` and `interfaces.environment` support `${}` references to wire components together.

### Reference Syntax

```
${<section>.<name>.<attribute>}
```

### Available References

| Reference                                      | Resolves to                                                  |
| ---------------------------------------------- | ------------------------------------------------------------ |
| `${<section>.<name>.host}`                     | Service DNS for the named component.                         |
| `${<section>.<name>.<endpoint>.port}`          | Port number (string) for the named endpoint.                 |
| `${<section>.<name>.<endpoint>.url}`           | `<protocol>://<host>:<port>` for the named endpoint.         |
| `${variables.<KEY>}`                           | Variable value (resolved from Secret at runtime).            |
| `${source.name}`                               | Agent name.                                                  |
| `${source.build}`                              | Build identifier.                                            |

`<section>` is one of `models`, `knowledge`, or `tools`. The `host` reference is per-component (all endpoints share the same service DNS). The `port` and `url` references require an endpoint name.

### Platform-Managed Environment Variables

The translator MUST inject the following environment variables regardless of `agent.environment` content:

- `GRPC_SERVER_ADDR` — injected when `interfaces` is present (i.e. messaging is enabled).
- `OTEL_EXPORTER_OTLP_ENDPOINT` — injected when `observability.enabled` is `true`.

These MAY appear in the template for visibility but are platform-managed and MUST NOT be overridden by user configuration.

---

## 12. Template Fields (`deployment-template/v1`)

The following fields are present only in `deployment-template/v1`. They MUST be stripped during fulfillment and MUST NOT appear in `deployment/v1`.

### 12.1 Template Variable Fields

Additional fields on each `variables` entry. `default` replaces `value` in the template — it is the pre-filled suggestion shown to the user in the UI.

| Field        | Type     | Required     | Description                                                                    |
| ------------ | -------- | ------------ | ------------------------------------------------------------------------------ |
| `default`    | string   | **REQUIRED** | Pre-filled value shown in the UI. The user replaces this with their actual value at fulfillment, which becomes `value` in `deployment/v1`. |
| `description`| string   | OPTIONAL     | Human-readable description shown in the UI.                                    |
| `datatype`   | string   | OPTIONAL     | Value type. MUST be one of: `string`, `boolean`, `number`, `array`, `object`.  |
| `display-as` | string   | OPTIONAL     | UI rendering hint. MUST be one of: `short-text`, `long-text`, `select`.        |
| `options`    | string[] | OPTIONAL     | Allowed values for `select` display. REQUIRED when `display-as` is `select`.   |

### 12.2 Editable Fields

The `editable` field lists field paths the user MAY modify during fulfillment. All fields not in this list are server-owned and MUST be rejected if changed.

Paths use dot notation: `agent.replicas`, `agent.resources`, `agent.endpoints.*.expose`. Map entries use `*` as a wildcard for the key: `variables.*.value` means the `value` field of every variable entry is editable.

---

## 13. Shared Sub-Schemas

### 13.1 Resources

CPU and memory requests and limits. All fields OPTIONAL — the server applies defaults based on component type when omitted.

| Field          | Type   | Required | Description                              |
| -------------- | ------ | -------- | ---------------------------------------- |
| `cpu`          | string | OPTIONAL | CPU request (e.g. `100m`, `2`).          |
| `memory`       | string | OPTIONAL | Memory request (e.g. `256Mi`, `8Gi`).    |
| `cpu_limit`    | string | OPTIONAL | CPU limit (e.g. `1`, `4`).               |
| `memory_limit` | string | OPTIONAL | Memory limit (e.g. `1Gi`, `16Gi`).       |

### 13.2 GPUConfig

Extends the AstroAI Spec GPUConfig with `count` for multi-GPU scheduling.

| Field     | Type    | Required | Description                                                        |
| --------- | ------- | -------- | ------------------------------------------------------------------ |
| `vram`    | string  | OPTIONAL | GPU memory scheduling hint (e.g. `24Gi`).                          |
| `runtime` | string  | OPTIONAL | GPU runtime. MUST be one of `cuda` or `rocm`. Default: `cuda`.    |
| `count`   | integer | OPTIONAL | Number of GPUs (deployment-spec extension). Default: `1`.          |

### 13.3 StorageConfig

| Field         | Type   | Required | Description                                                              |
| ------------- | ------ | -------- | ------------------------------------------------------------------------ |
| `size`        | string | OPTIONAL | PVC size (e.g. `10Gi`). Default: `10Gi`.                                |
| `class`       | string | OPTIONAL | Storage class name (e.g. `gp3`, `io1`). Omit for cluster default.       |
| `access_mode` | string | OPTIONAL | MUST be `ReadWriteOnce` or `ReadWriteMany`. Default: `ReadWriteOnce`.   |

### 13.4 Healthcheck

Defines both liveness and readiness probes. The runtime MUST create identical probes from this configuration. Extends the AstroAI Spec Healthcheck with `initial_delay` for runtime probe scheduling.

| Field           | Type     | Required | Description                                                       |
| --------------- | -------- | -------- | ----------------------------------------------------------------- |
| `test`          | string[] | OPTIONAL | Exec probe command (e.g. `["CMD", "redis-cli", "ping"]`).        |
| `path`          | string   | OPTIONAL | HTTP GET probe path (e.g. `/health`).                             |
| `initial_delay` | string   | OPTIONAL | Delay before first check (deployment-spec extension). Default: `10s`. |
| `interval`      | string   | OPTIONAL | Check frequency. Default: `10s`.                                  |
| `timeout`       | string   | OPTIONAL | Per-check timeout. Default: `5s`.                                 |
| `retries`       | integer  | OPTIONAL | Consecutive failures before unhealthy. Default: `3`.              |

When neither `test` nor `path` is set, the server SHOULD generate a provider-appropriate probe during template generation.

### 13.5 UpdateStrategy

Controls how changes are rolled out. Applicable to long-running workloads (agent, models, knowledge, tools, webhook ingestion). MUST be ignored for jobs and cronjobs.

| Field             | Type   | Required | Description                                                         |
| ----------------- | ------ | -------- | ------------------------------------------------------------------- |
| `strategy`        | string | OPTIONAL | MUST be `rolling` or `recreate`. Default: `rolling`.                |
| `max_unavailable` | string | OPTIONAL | Max pods down during rolling update. Default: `25%`. Ignored when `strategy` is `recreate`. |
| `max_surge`       | string | OPTIONAL | Max extra pods during rolling update. Default: `25%`. Ignored when `strategy` is `recreate`. |

### 13.6 Endpoint

A named network endpoint that a component serves. Each entry in an `endpoints` map declares a port, its protocol, and optional external exposure.

| Field      | Type          | Required     | Description                                                              |
| ---------- | ------------- | ------------ | ------------------------------------------------------------------------ |
| `port`     | integer       | **REQUIRED** | Port number the endpoint listens on.                                     |
| `protocol` | string        | OPTIONAL     | Protocol served. MUST be one of `http`, `grpc`, or `tcp`. Default: `http`. |
| `expose`   | EndpointExpose | OPTIONAL    | External access configuration.                                           |

#### EndpointExpose

| Field     | Type    | Required | Description                                              |
| --------- | ------- | -------- | -------------------------------------------------------- |
| `enabled` | boolean | OPTIONAL | Whether to create external access (ingress). Default: `false`. |
| `domain`  | string  | OPTIONAL | Domain for external access.                              |

---

## 14. Validation Rules

Implementations MUST enforce the following rules:

1. `spec` MUST be `deployment-template/v1` or `deployment/v1`.
2. `source.name`, `source.build`, and `source.registry` MUST be non-empty strings.
3. `target.runtime` MUST be `kubernetes`.
4. `target.namespace` MUST be a non-empty string valid for the target runtime.
5. `agent.image` MUST be a non-empty string and `agent.endpoints` MUST contain at least one entry.
6. For each entry in `models`, `knowledge`, and `tools`: `image` MUST be a non-empty string, `endpoints` MUST contain at least one entry, and each endpoint `port` MUST be a positive integer.
6a. When `endpoint.protocol` is provided, it MUST be one of `http`, `grpc`, or `tcp`.
7. For each entry in `knowledge`: when `persistent` is `true`, `storage` MUST be present.
8. For each entry in `ingestion`: `image` MUST be a non-empty string, `trigger` MUST be present, and `trigger.type` MUST be one of `schedule`, `startup`, `manual`, `webhook`.
9. When `trigger.type` is `schedule`, `trigger.schedule` MUST be a non-empty string containing a valid cron expression.
10. When `trigger.type` is NOT `schedule`, `trigger.schedule` MUST NOT be present.
11. When `interfaces` is present: `adapters` MUST be a non-empty array, and `image` MUST be a non-empty string.
12. In `deployment/v1`, for each entry in `variables` where `optional` is `false` or absent: `value` MUST be a non-empty string. `value` is stripped from storage when `secret` is `true`. `deployment/v1` MUST NOT contain `default` on any `variables` entry.
12a. `variables.*.targets` MUST be a non-empty array. Each element MUST be `agent`, `ingestion`, `ingestion.<name>` where `<name>` is a key in `ingestion`, or `interface.<adapter>` where `<adapter>` is a name listed in `interfaces.adapters`.
12b. When `variables.*.display-as` is `select`, `variables.*.options` MUST be present and non-empty.
12c. When `variables.*.datatype` is provided, it MUST be one of `string`, `boolean`, `number`, `array`, `object`.
12d. When `variables.*.display-as` is provided, it MUST be one of `short-text`, `long-text`, `select`.
13. All `${}` references in `agent.environment` and `interfaces.environment` MUST resolve to a declared component, variable, or source attribute. A reference to a non-existent entry (e.g. `${models.foo.url}` when no model `foo` exists) is invalid.
14. There MUST NOT be duplicate ports within the same deployment scope.
15. When `gpu.runtime` is provided, it MUST be one of `cuda` or `rocm`.
16. When `storage.access_mode` is provided, it MUST be one of `ReadWriteOnce` or `ReadWriteMany`.
17. When `update.strategy` is provided, it MUST be one of `rolling` or `recreate`.
18. When `agent.distributed` is `false` or absent, `agent.replicas` MUST be `1`.
19. During fulfillment, any field changed that is not in the `editable` list MUST be rejected.
20. A `deployment/v1` document MUST NOT contain an `editable` field.
21. A `deployment/v1` document MUST NOT contain template-only variable fields (`description`, `datatype`, `display-as`, `options`, `default`) on any `variables` entry.

---

## Appendix A: Template Generation and API Surface (Non-Normative)

### Template Generation Endpoint

```
GET /api/v1/agents/:name/:version/deployment-template
```

Returns a deployment spec YAML template with placeholder values and descriptions.

**Steps:**

1. Fetch registered AstroAI Spec from the agent index.
2. For each self-hosted model: resolve provider to image and port. For each cloud model: extract variables only.
3. Same for knowledge and tools.
4. Populate `agent.environment` with `${}` references using conventional env var names.
5. Extract variables from all referenced providers and inputs, emitting one variable entry per declared variable. All `Input` fields carry through transparently. Set `targets` based on the variable's origin: provider variables → `[agent]`; interface variables → `[interface.<adapter>]` for the adapter that declared them; ingestion-level inputs → `[ingestion.<name>]` for the specific pipeline; top-level inputs → `[agent, ingestion]` plus one `interface.<adapter>` entry per enabled adapter; `agent.inputs` → `[agent]`.
6. For each `schedule`-type ingestion: emit `schedule: ""` placeholder.
7. If `agent.interfaces.messaging` is `true` (or `interfaces` is omitted): emit `interfaces` block with `adapters: []` and the platform messaging sidecar image. Otherwise omit the block.
7a. If `agent.interfaces.frontend` is `true`: set the agent's HTTP endpoint to port 80 with `expose.enabled: true`.
8. Apply defaults: `replicas: 1`, `observability.enabled: true`, `expose.enabled: false` on non-frontend endpoints, `target.runtime: kubernetes`.
9. Set `source` fields from registered spec metadata.
10. Emit `editable` list.

### Deploy Endpoint

```
POST /api/v1/deploy
Content-Type: application/yaml
Body: filled-in deployment spec
```

The server parses, validates editable enforcement, resolves defaults, stores (variable values stripped), translates, and applies.

### Fulfillment and Validation

- **Editable enforcement** — re-generate the template for the same agent version. Any field changed that is not in the `editable` list is rejected.
- Validate all `${}` references resolve to declared entries.
- Validate all required variables have non-empty values.
- Validate cron expressions for `schedule`-type ingestion triggers.
- Validate `target.namespace` for the target runtime.
- Check for duplicate ports.

### Storage

The fulfilled deployment spec (variable values stripped) is stored alongside the agent version. This provides an audit trail, enables redeploy, diff, and rollback.

---

## Appendix B: Resource Defaults (Non-Normative)

Default resource values applied by the server when omitted:

| Component Type                          | CPU Request | Memory Request | CPU Limit | Memory Limit |
| --------------------------------------- | ----------- | -------------- | --------- | ------------ |
| Standard (agent, tools, non-GPU models) | `100m`      | `256Mi`        | `1`       | `1Gi`        |
| GPU workloads (models with `gpu`)       | `2`         | `8Gi`          | `4`       | `16Gi`       |
| Messaging sidecars                      | `100m`      | `128Mi`        | `500m`    | `512Mi`      |
| Collector                               | `50m`       | `128Mi`        | `250m`    | `256Mi`      |

---

## Appendix C: Complete Examples

### C.1 Template (`deployment-template/v1`)

```yaml
spec: deployment-template/v1

source:
  name: engineering-assistant
  build: "7"
  registry: registry.astro.dev/acme

target:
  runtime: kubernetes
  namespace: ""

agent:
  image: registry.astro.dev/acme/engineering-assistant:2.0.0
  endpoints:
    http:
      port: 8080
  replicas: 1
  resources:
    cpu: "100m"
    memory: "256Mi"
    cpu_limit: "1"
    memory_limit: "1Gi"
  environment:
    MODEL_LOCAL_LLM_HOST: "${models.local_llm.host}"
    MODEL_LOCAL_LLM_PORT: "${models.local_llm.http.port}"
    MODEL_LOCAL_LLM_URL: "${models.local_llm.http.url}"
    QDRANT_HOST: "${knowledge.docs.host}"
    QDRANT_PORT: "${knowledge.docs.http.port}"
    QDRANT_URL: "${knowledge.docs.http.url}"
    ANTHROPIC_API_KEY: "${variables.ANTHROPIC_API_KEY}"
    GITHUB_TOKEN: "${variables.GITHUB_TOKEN}"
    ASTRO_AGENT_NAME: "${source.name}"
    ASTRO_AGENT_BUILD: "${source.build}"
  update:
    strategy: rolling
    max_unavailable: "25%"
    max_surge: "25%"

models:
  local_llm:
    image: ollama/ollama:latest
    endpoints:
      http:
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
    image: qdrant/qdrant:latest
    endpoints:
      http:
        port: 6333
      grpc:
        port: 6334
        protocol: grpc
    replicas: 1
    resources:
      cpu: "100m"
      memory: "256Mi"
      cpu_limit: "1"
      memory_limit: "1Gi"
    persistent: true
    storage:
      size: "10Gi"
      class: ""
      access_mode: ReadWriteOnce
    healthcheck:
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
      schedule: "0 */6 * * *"
    environment:
      SOURCE_REPO: company/engineering-docs
      TARGET_COLLECTION: docs

interfaces:
  adapters: [slack, web]
  image: registry.astro.dev/prod-astro-messaging:latest
  endpoints:
    grpc:
      port: 9090
      protocol: grpc
    http:
      port: 8080
      expose:
        enabled: true
        domain: ""
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
    SLACK_BOT_TOKEN: "${variables.SLACK_BOT_TOKEN}"
    SLACK_APP_TOKEN: "${variables.SLACK_APP_TOKEN}"
    WEB_ENABLED: "true"
    WEB_LISTEN_ADDR: ":8080"

variables:
  ANTHROPIC_API_KEY:
    default: ""
    targets: [agent]
    description: Anthropic API key for Claude models
    secret: true
    optional: false
  GITHUB_TOKEN:
    default: ""
    targets: [agent]
    description: GitHub token for API access
    secret: true
    optional: false
  SLACK_BOT_TOKEN:
    default: ""
    targets: [interface.slack]
    description: Slack bot token for messaging
    secret: true
    optional: false
  SLACK_APP_TOKEN:
    default: ""
    targets: [interface.slack]
    description: Slack app token for socket mode
    secret: true
    optional: false

observability:
  enabled: true
  provider: galileo

editable:
  - target.namespace
  - agent.endpoints.*.expose
  - ingestion.*.trigger.schedule
  - interfaces.adapters
  - interfaces.endpoints.*.expose
  - variables.*.value
  - variables.*.targets
  - observability.enabled
```

### C.2 Fulfilled Spec (`deployment/v1`)

The same agent after the user has filled in the template. Template-only fields (`editable`, variable UI metadata) are stripped; `default` becomes `value`.

```yaml
spec: deployment/v1

source:
  name: engineering-assistant
  build: "7"
  registry: registry.astro.dev/acme

target:
  runtime: kubernetes
  namespace: engineering

agent:
  image: registry.astro.dev/acme/engineering-assistant:2.0.0
  endpoints:
    http:
      port: 8080
  replicas: 2
  resources:
    cpu: "100m"
    memory: "256Mi"
    cpu_limit: "1"
    memory_limit: "1Gi"
  environment:
    MODEL_LOCAL_LLM_HOST: "${models.local_llm.host}"
    MODEL_LOCAL_LLM_PORT: "${models.local_llm.http.port}"
    MODEL_LOCAL_LLM_URL: "${models.local_llm.http.url}"
    QDRANT_HOST: "${knowledge.docs.host}"
    QDRANT_PORT: "${knowledge.docs.http.port}"
    QDRANT_URL: "${knowledge.docs.http.url}"
    ANTHROPIC_API_KEY: "${variables.ANTHROPIC_API_KEY}"
    GITHUB_TOKEN: "${variables.GITHUB_TOKEN}"
    ASTRO_AGENT_NAME: "${source.name}"
    ASTRO_AGENT_BUILD: "${source.build}"
  update:
    strategy: rolling
    max_unavailable: "25%"
    max_surge: "25%"

models:
  local_llm:
    image: ollama/ollama:latest
    endpoints:
      http:
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
    image: qdrant/qdrant:latest
    endpoints:
      http:
        port: 6333
      grpc:
        port: 6334
        protocol: grpc
    replicas: 1
    resources:
      cpu: "100m"
      memory: "256Mi"
      cpu_limit: "1"
      memory_limit: "1Gi"
    persistent: true
    storage:
      size: "10Gi"
      class: ""
      access_mode: ReadWriteOnce
    healthcheck:
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
      schedule: "0 */6 * * *"
    environment:
      SOURCE_REPO: company/engineering-docs
      TARGET_COLLECTION: docs

interfaces:
  adapters: [slack, web]
  image: registry.astro.dev/prod-astro-messaging:latest
  endpoints:
    grpc:
      port: 9090
      protocol: grpc
    http:
      port: 8080
      expose:
        enabled: true
        domain: assistant.acme.dev
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
    SLACK_BOT_TOKEN: "${variables.SLACK_BOT_TOKEN}"
    SLACK_APP_TOKEN: "${variables.SLACK_APP_TOKEN}"
    WEB_ENABLED: "true"
    WEB_LISTEN_ADDR: ":8080"

variables:
  ANTHROPIC_API_KEY:
    value: sk-ant-...
    targets: [agent]
    secret: true
  GITHUB_TOKEN:
    value: ghp_...
    targets: [agent]
    secret: true
  SLACK_BOT_TOKEN:
    value: xoxb-...
    targets: [interface.slack]
    secret: true
  SLACK_APP_TOKEN:
    value: xapp-...
    targets: [interface.slack]
    secret: true

observability:
  enabled: true
  provider: galileo
```
