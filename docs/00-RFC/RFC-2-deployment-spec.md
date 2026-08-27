# RFC-2: Astro Deployment Spec (deployment/v1)

| | |
|---|---|
| **Version** | 2.0 (Draft) |
| **Date** | 2026-02-18 |
| **Authors** | Saswat Das ([@saswatds](https://github.com/saswatds)) |

## Abstract

The Astro Deployment Spec (`deployment/v1`) is a declarative format for describing how to deploy a specific version of an agent. It contains the runtime fields needed to produce infrastructure manifests — concrete images, ports, variables, and component wiring. It sits between the Astropods Spec (agent topology) and infrastructure manifests (runtime resources). A conforming spec deterministically produces identical infrastructure manifests.

This RFC covers only the deployment spec format itself.

## Changelog

| Version | Date       | Changes        |
| ------- | ---------- | -------------- |
| v1.0    | 2026-02-18 | Initial draft. |
| v2.0    | 2026-04-17 | Refocus on deployment/v1 only. |
| v2.1    | 2026-08-26 | Corrected against implementation: added the missing `integrations` section, `target.cluster_id`, and `agent.volume`/`storage`/`astro_ai_gateway`/`response_timeout`; fixed the knowledge-binding, `InterfacesAuth`, `target.runtime`, and port-uniqueness sections to match actual validation; fixed 3 of 4 rows in Appendix B's resource defaults. No design changes — this version reflects what already shipped. |

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Introduction

The deployment spec occupies the middle stage of a three-stage pipeline:

```
Astropods Spec → Deployment Spec → Infrastructure Manifests
```

The Astropods Spec declares agent topology using provider bindings — components reference named providers that the platform classifies as self-hosted or cloud. The deployment spec eliminates this indirection: self-hosted providers become concrete images and ports, cloud and custom providers become variable entries, and container-mode entries carry through directly. The result is a document with no provider names — only images, ports, and variables.

The deployment spec is posted to the deploy endpoint for translation into infrastructure manifests.

**Translation** is a deterministic structural mapping from the spec to target infrastructure manifests. The translator consumes only the deployment spec — no provider lookups, no variable discovery. Because the spec describes workloads in runtime-agnostic terms (containers, ports, replicas, persistence, scheduling), different translators can target different platforms. A Kubernetes translator produces Deployments, Services, and CronJobs; an ECS translator could produce Task Definitions and Services from the same spec. The `target.runtime` field selects which translator is used.

### Scope

This spec covers the schema and validation rules for `deployment/v1` documents. It does NOT cover how the spec is produced, translation logic, or infrastructure manifest formats — those are implementation concerns.

---

## 2. Top-Level Structure

A conforming document MUST contain the following top-level fields:

| Field           | Type                          | Required     | Description                                                                              |
| --------------- | ----------------------------- | ------------ | ---------------------------------------------------------------------------------------- |
| `spec`          | string                        | **REQUIRED** | Spec version identifier. MUST be `deployment/v1`.                                        |
| `source`        | object                        | **REQUIRED** | Agent source metadata (Section 3).                                                       |
| `target`        | object                        | **REQUIRED** | Deployment target (Section 4).                                                           |
| `agent`         | object                        | **REQUIRED** | Agent container configuration (Section 5).                                               |
| `models`        | map\<string, ModelEntry\>     | OPTIONAL     | Self-hosted model entries (Section 6.1).                                                 |
| `knowledge`     | map\<string, KnowledgeEntry\> | OPTIONAL     | Knowledge store entries (Section 6.2).                                                   |
| `integrations`  | map\<string, IntegrationEntry\> | OPTIONAL   | Integration sidecar entries (Section 6.3).                                               |
| `ingestion`     | map\<string, IngestionEntry\> | OPTIONAL     | Ingestion pipeline entries (Section 7).                                                  |
| `interfaces`    | object                        | OPTIONAL     | Messaging sidecar configuration (Section 8). Only present when the agent supports messaging. |
| `variables`     | map\<string, Variable\>       | OPTIONAL     | Deployment variables (Section 9).                                                        |
| `observability` | object                        | OPTIONAL     | Observability configuration (Section 10).                                                |

Cloud providers from the Astropods Spec do NOT appear in `models` or `knowledge`. They are represented solely as `variables` entries.

---

## 3. Source

The `source` object identifies the agent and build being deployed.

| Field      | Type   | Required     | Description                              |
| ---------- | ------ | ------------ | ---------------------------------------- |
| `account`  | string | **REQUIRED** | Account that owns the agent.             |
| `name`     | string | **REQUIRED** | Agent name from the Astropods Spec.      |
| `build`    | string | **REQUIRED** | Build identifier.                        |
| `registry` | string | **REQUIRED** | Registry where agent images were pushed. |

---

## 4. Target

The `target` object specifies where the deployment runs.

| Field           | Type   | Required     | Description                                                                        |
| --------------- | ------ | ------------ | ---------------------------------------------------------------------------------- |
| `runtime`       | string | OPTIONAL     | Target runtime. If present, MUST be `kubernetes` — the only implemented value. An absent or empty value is accepted; implementations MAY add additional runtimes later. |
| `account`       | string | OPTIONAL     | Target account for cross-account deploys. Defaults to `source.account`.            |
| `display_name`  | string | OPTIONAL     | Human-readable deployment name. Must be unique within the account.                 |
| `deployment_id` | string | OPTIONAL     | Existing deployment ID for in-place updates.                                       |
| `cluster_id`    | string | OPTIONAL     | Cluster to route the deployment to. Implementation-internal — set by the server, not authored by the caller. |

---

## 5. Agent

The `agent` object configures the primary agent container.

| Field         | Type                    | Required     | Description                                                     |
| ------------- | ----------------------- | ------------ | --------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                             |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints the agent serves (Section 12.6).        |
| `distributed` | boolean                 | OPTIONAL     | Whether the agent supports multiple replicas. Default: `false`. |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                               |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                    |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables. Supports `${}` references (Section 11).  |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                      |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 12.5).                                |
| `volume`      | string                  | OPTIONAL     | Mount path for persistent storage. A non-empty value switches the agent from an ephemeral Deployment to a StatefulSet with a PVC. Default: empty (ephemeral). |
| `storage`     | StorageConfig           | OPTIONAL     | PVC configuration (Section 12.3), applied only when `volume` is set. Defaults applied if omitted. |
| `astro_ai_gateway` | boolean            | OPTIONAL     | Whether to mint an Astro AI Gateway virtual key for this agent and inject `ASTRO_GATEWAY_URL`/`ASTRO_GATEWAY_API_KEY`. Rejected at admission when the gateway isn't configured server-side. Default: `false`. |
| `response_timeout` | string             | OPTIONAL     | How long the front-door proxy waits for a complete response before returning 504. A Go duration string (`"15s"`, `"2m"`) bounded by an implementation-defined maximum. Default: implementation-defined. |

When the agent declares `interfaces.frontend: true` in the Astropods Spec, the HTTP endpoint is set to port 80 with `expose.enabled: true`. This creates an ingress directly to the agent container, bypassing the messaging sidecar.

---

## 6. Component Sections: Models, Knowledge

Components represent self-hosted services deployed alongside the agent. All provider resolution has already occurred — entries contain concrete images and ports, not provider names.

### 6.1 Models

Each entry in the `models` map configures a self-hosted model container.

| Field         | Type                    | Required     | Description                                                                        |
| ------------- | ----------------------- | ------------ | ---------------------------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                                                |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 12.6).                                            |
| `model`       | string                  | OPTIONAL     | Provider-specific model identifier (e.g. `llama3.2`). Carried from Astropods Spec. |
| `persistent`  | boolean                 | OPTIONAL     | Whether to persist model data (e.g. pulled weights). Default: `false`.             |
| `provider`    | string                  | OPTIONAL     | Provider name (implementation-internal).                                           |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                                                  |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                                       |
| `gpu`         | GPUConfig               | OPTIONAL     | GPU resource requirements (Section 12.2).                                          |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the model container.                                     |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                                         |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 12.5).                                                   |

### 6.2 Knowledge

Each entry in the `knowledge` map configures a knowledge store container.

| Field         | Type                    | Required     | Description                                                             |
| ------------- | ----------------------- | ------------ | ----------------------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                                     |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 12.6).                                 |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                                       |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                            |
| `persistent`  | boolean                 | OPTIONAL     | Whether to use persistent storage (StatefulSet). Default: `false`.      |
| `volume`      | string                  | OPTIONAL     | Mount path for persistent storage.                                      |
| `storage`     | StorageConfig           | OPTIONAL     | PVC configuration (Section 12.3). REQUIRED when `persistent` is `true`. |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the knowledge store container.                |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                              |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 12.5).                                        |
| `provider`    | string                  | OPTIONAL     | Provider name (implementation-internal).                                |
| `binding`     | string                  | OPTIONAL     | ARN of a managed store this entry is bound to. When set, every other field above is zero-valued and unused — connection details resolve from the store record at deploy time, not from this entry. |

A knowledge entry bound to a managed store **stays in this map** with only `binding` set; it is not removed and not resolved into `agent.environment` env vars. `${knowledge.<name>.host}` references still work identically for bound and self-hosted entries — the resolver just reads a different source depending on whether `binding` is set.

### 6.3 Integrations

Each entry in the `integrations` map configures an integration sidecar container, following the same shape as a model or knowledge entry.

| Field         | Type                    | Required     | Description                                                |
| ------------- | ----------------------- | ------------ | ----------------------------------------------------------- |
| `image`       | string                  | **REQUIRED** | Resolved container image reference.                         |
| `endpoints`   | map\<string, Endpoint\> | **REQUIRED** | Named network endpoints (Section 12.6).                      |
| `replicas`    | integer                 | OPTIONAL     | Number of replicas. Default: `1`.                            |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                 |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the integration container.         |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                   |
| `update`      | UpdateStrategy          | OPTIONAL     | Rollout strategy (Section 12.5).                              |

---

## 7. Ingestion

Each entry in the `ingestion` map configures a data ingestion pipeline. Ingestion entries are flattened from the Astropods Spec's `container` object: `image` replaces `container.image`, and `endpoints` replaces the single `container.port`.

| Field         | Type                    | Required     | Description                                                                          |
| ------------- | ----------------------- | ------------ | ------------------------------------------------------------------------------------ |
| `image`       | string                  | **REQUIRED** | Container image for the ingestion pipeline.                                          |
| `endpoints`   | map\<string, Endpoint\> | OPTIONAL     | Named network endpoints (Section 12.6). Applicable when `trigger.type` is `webhook`. |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                                         |
| `trigger`     | IngestionTrigger        | **REQUIRED** | Trigger configuration (Section 7.1).                                                 |
| `environment` | map\<string, string\>   | OPTIONAL     | Environment variables for the ingestion container.                                   |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                                           |

### 7.1 Ingestion Trigger

| Field      | Type   | Required     | Description                                                                         |
| ---------- | ------ | ------------ | ----------------------------------------------------------------------------------- |
| `type`     | string | **REQUIRED** | MUST be one of: `schedule`, `startup`, `manual`, `webhook`.                         |
| `schedule` | string | Conditional  | Cron expression. REQUIRED when `type` is `schedule`. MUST NOT be present otherwise. |

---

## 8. Interfaces

The `interfaces` object configures messaging adapters (e.g. Slack, web) deployed as a sidecar, and/or access grants for the agent's own custom web interface. Most fields below apply only to the messaging sidecar; `auth.custom` is the exception — it can be the only thing set, for a frontend-only agent with no messaging sidecar at all.

| Field         | Type                    | Required     | Description                                                        |
| ------------- | ----------------------- | ------------ | ------------------------------------------------------------------ |
| `adapters`    | string[]                | Conditional  | Enabled adapter names (e.g. `["slack", "web"]`). REQUIRED when this block configures a messaging sidecar (i.e. `adapters` or `image` is set); absent when `interfaces` carries only `auth.custom`. |
| `image`       | string                  | Conditional  | Messaging sidecar container image. REQUIRED under the same condition as `adapters`. |
| `endpoints`   | map\<string, Endpoint\> | OPTIONAL     | Named network endpoints (Section 12.6). Required in practice for the `web` adapter, which needs an exposed `http` endpoint. |
| `resources`   | Resources               | OPTIONAL     | CPU and memory configuration (Section 12.1).                       |
| `environment` | map\<string, string\>   | OPTIONAL     | Adapter-specific environment variables. Supports `${}` references. |
| `healthcheck` | Healthcheck             | OPTIONAL     | Health check configuration (Section 12.4).                         |
| `auth`        | InterfacesAuth          | OPTIONAL     | Per-adapter authentication and access-grant configuration.         |

#### InterfacesAuth

| Field    | Type       | Required | Description                                                                    |
| -------- | ---------- | -------- | ------------------------------------------------------------------------------ |
| `web`    | WebAuth    | OPTIONAL | Authentication and access grants for the web adapter ingress. Nil means no auth, no grants. |
| `slack`  | SlackAuth  | OPTIONAL | Access grants for the slack adapter. Nil means no grants.                      |
| `custom` | CustomAuth | OPTIONAL | Access grants for the agent's own custom web interface (distinct from the platform's `web` messaging chat). Nil means no grants. |

#### WebAuth

| Field    | Type                            | Required | Description                                                                              |
| -------- | ------------------------------- | -------- | ---------------------------------------------------------------------------------------- |
| `type`   | string                          | OPTIONAL | Authentication type. `oidc` uses server-level OIDC config. `oidc-custom` is reserved for future per-deployment credentials. |
| `public` | boolean                         | OPTIONAL | Routes the web chat ingress to the open (no-OIDC) cohort, bypassing front-door sign-in. Since the OIDC identity header is then absent, authorization falls to `grants` — an `anyone` grant is required for this to actually be reachable. Default: `false`. |
| `grants` | AuthorizationGrant[]             | OPTIONAL | Who may reach the web adapter. May include account, user, and anyone subjects (Section 8.1). |

#### SlackAuth

| Field    | Type                 | Required | Description                                                                    |
| -------- | -------------------- | -------- | ------------------------------------------------------------------------------ |
| `grants` | AuthorizationGrant[] | OPTIONAL | Who may use the slack adapter. `org`, `user_id`, and `anyone` grants are all accepted — a `user_id` grant resolves via a linked Slack identity mapping; a Slack user who hasn't linked their identity falls through to the account/anyone candidates instead of matching. |

#### CustomAuth

| Field    | Type                 | Required | Description                                                                    |
| -------- | -------------------- | -------- | ------------------------------------------------------------------------------ |
| `public` | boolean              | OPTIONAL | Routes the custom interface to the open (no-OIDC) cohort. Default: `false`.    |
| `grants` | AuthorizationGrant[] | OPTIONAL | Who may use the custom interface. Recorded for visibility but **not enforced by the platform** — the agent's own server is responsible for authorization. |

### 8.1 AuthorizationGrant

A single access grant. The adapter it applies to is implied by where it lives (`interfaces.auth.web.grants` vs. `.slack.grants` vs. `.custom.grants`), not carried on the grant itself. Exactly one field MUST be set:

| Field     | Type    | Required    | Description                                              |
| --------- | ------- | ----------- | --------------------------------------------------------- |
| `org`     | string  | Conditional | Any member of this organization (account) is allowed.     |
| `user_id` | string  | Conditional | This specific WorkOS user is allowed. Web adapter only.    |
| `anyone`  | boolean | Conditional | Anyone hitting the adapter is allowed.                     |

Adapter-specific behavioral configuration (e.g. actionable emoji reactions, socket mode, auto-threading) is passed to the messaging sidecar via the `SLACK_CONFIG` environment variable as a JSON object. This variable is injected into `interfaces.environment` through the variables mechanism and is editable in the deploy UI.

---

## 9. Variables

Each entry in the `variables` map declares a deployment variable. Variables are derived from three sources:

- **Providers** — each provider referenced by a component produces one variable entry per declared variable.
- **Interfaces** — messaging adapters (e.g. `slack`) produce variable entries for adapter-specific configuration.
- **Inputs** — `inputs` entries from the Astropods Spec (top-level, `agent.inputs`, and per-component) produce variable entries. These variables are referenced via `${variables.<name>}` in `environment` fields.

| Field      | Type     | Required     | Description                                                                                                                                                                                                          |
| ---------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `value`    | string   | Conditional  | The supplied value. REQUIRED for non-optional variables. Stripped before storage when `secret: true`.                                                                                                                |
| `ref`      | string   | OPTIONAL     | Reference to an account variable by name. Mutually exclusive with `value` — exactly one of `value` or `ref` MUST be present for non-optional variables.                                                             |
| `targets`  | string[] | **REQUIRED** | Containers this variable is injected into. Each value MUST be one of: `agent`, `ingestion` (all pipelines), `ingestion.<name>` (specific pipeline), or `interface.<adapter>` (e.g. `interface.slack`).               |
| `secret`   | boolean  | OPTIONAL     | Whether the value is sensitive. Default: `false`. Secret values are stored in a Secret and never logged. Non-secret values are injected as plain env vars.                                                           |
| `optional` | boolean  | OPTIONAL     | Whether deployment MAY proceed without this variable. Default: `false`.                                                                                                                                              |

---

## 10. Observability

The `observability` object configures monitoring and telemetry.

| Field         | Type                | Required | Description                                                                  |
| ------------- | ------------------- | -------- | ---------------------------------------------------------------------------- |
| `enabled`     | boolean             | OPTIONAL | Whether to deploy a collector sidecar. Default: `true`.                      |
| `provider`    | string              | OPTIONAL | Observability provider. Currently only `langfuse` is supported.              |
| `image`       | string              | OPTIONAL | Collector sidecar image (implementation-internal).                           |
| `port`        | integer             | OPTIONAL | OTLP receiver port (implementation-internal). Default: `4318`.               |
| `resources`   | Resources           | OPTIONAL | CPU and memory configuration (Section 12.1). Implementation-internal.        |
| `environment` | map\<string, string\> | OPTIONAL | Collector environment variables. Implementation-internal.                  |
| `log_stream`  | string              | OPTIONAL | Provider-specific log stream name. Default: `{source.name}-{deployment_id}`. |

---

## 11. Component References

Environment variables in `agent.environment` and `interfaces.environment` support `${}` references to wire components together.

### Reference Syntax

```
${<section>.<name>.<attribute>}
```

### Available References

| Reference                             | Resolves to                                          |
| ------------------------------------- | ---------------------------------------------------- |
| `${<section>.<name>.host}`            | Service DNS for the named component.                 |
| `${<section>.<name>.<endpoint>.port}` | Port number (string) for the named endpoint.         |
| `${<section>.<name>.<endpoint>.url}`  | `<protocol>://<host>:<port>` for the named endpoint. |
| `${variables.<KEY>}`                  | Variable value (resolved from Secret at runtime).    |
| `${source.name}`                      | Agent name.                                          |
| `${source.build}`                     | Build identifier.                                    |

`<section>` is one of `models`, `knowledge`, or `integrations`. The `host` reference is per-component (all endpoints share the same service DNS). The `port` and `url` references require an endpoint name.

### Platform-Managed Environment Variables

The translator MUST inject the following environment variables regardless of `agent.environment` content:

- `GRPC_SERVER_ADDR` — injected when `interfaces` is present (i.e. messaging is enabled).
- `OTEL_EXPORTER_OTLP_ENDPOINT` — injected when `observability.enabled` is `true`.

These are platform-managed and MUST NOT be overridden by user configuration.

---

## 12. Shared Sub-Schemas

### 12.1 Resources

CPU and memory requests and limits. All fields OPTIONAL — the server applies defaults based on component type when omitted.

| Field          | Type   | Required | Description                           |
| -------------- | ------ | -------- | ------------------------------------- |
| `cpu`          | string | OPTIONAL | CPU request (e.g. `100m`, `2`).       |
| `memory`       | string | OPTIONAL | Memory request (e.g. `256Mi`, `8Gi`). |
| `cpu_limit`    | string | OPTIONAL | CPU limit (e.g. `1`, `4`).            |
| `memory_limit` | string | OPTIONAL | Memory limit (e.g. `1Gi`, `16Gi`).    |

### 12.2 GPUConfig

Extends the Astropods Spec GPUConfig with `count` for multi-GPU scheduling.

| Field     | Type    | Required | Description                                                    |
| --------- | ------- | -------- | -------------------------------------------------------------- |
| `vram`    | string  | OPTIONAL | GPU memory scheduling hint (e.g. `24Gi`).                      |
| `runtime` | string  | OPTIONAL | GPU runtime. MUST be one of `cuda` or `rocm`. Default: `cuda`. |
| `count`   | integer | OPTIONAL | Number of GPUs (deployment-spec extension). Default: `1`.      |

### 12.3 StorageConfig

| Field         | Type   | Required | Description                                                           |
| ------------- | ------ | -------- | --------------------------------------------------------------------- |
| `size`        | string | OPTIONAL | PVC size (e.g. `10Gi`). Default: `10Gi`.                              |
| `class`       | string | OPTIONAL | Storage class name (e.g. `gp3`, `io1`). Omit for cluster default.     |
| `access_mode` | string | OPTIONAL | MUST be `ReadWriteOnce` or `ReadWriteMany`. Default: `ReadWriteOnce`. |

### 12.4 Healthcheck

Defines both liveness and readiness probes. The runtime MUST create identical probes from this configuration.

| Field      | Type     | Required | Description                                                |
| ---------- | -------- | -------- | ---------------------------------------------------------- |
| `test`     | string[] | OPTIONAL | Exec probe command (e.g. `["CMD", "redis-cli", "ping"]`).  |
| `path`     | string   | OPTIONAL | HTTP GET probe path (e.g. `/health`).                      |
| `interval` | string   | OPTIONAL | Check frequency. Default: `10s`.                           |
| `timeout`  | string   | OPTIONAL | Per-check timeout. Default: `5s`.                          |
| `retries`  | integer  | OPTIONAL | Consecutive failures before unhealthy. Default: `3`.       |

When neither `test` nor `path` is set, the server SHOULD generate a provider-appropriate probe.

### 12.5 UpdateStrategy

Controls how changes are rolled out. Applicable to long-running workloads (agent, models, knowledge, webhook ingestion). MUST be ignored for jobs and cronjobs.

| Field             | Type   | Required | Description                                                                                  |
| ----------------- | ------ | -------- | -------------------------------------------------------------------------------------------- |
| `strategy`        | string | OPTIONAL | MUST be `rolling` or `recreate`. Default: `rolling`.                                         |
| `max_unavailable` | string | OPTIONAL | Max pods down during rolling update. Default: `25%`. Ignored when `strategy` is `recreate`.  |
| `max_surge`       | string | OPTIONAL | Max extra pods during rolling update. Default: `25%`. Ignored when `strategy` is `recreate`. |

### 12.6 Endpoint

A named network endpoint that a component serves. Each entry in an `endpoints` map declares a port, its protocol, and optional external exposure.

| Field      | Type           | Required     | Description                                                                |
| ---------- | -------------- | ------------ | -------------------------------------------------------------------------- |
| `port`     | integer        | **REQUIRED** | Port number the endpoint listens on.                                       |
| `protocol` | string         | OPTIONAL     | Protocol served. MUST be one of `http`, `grpc`, or `tcp`. Default: `http`. |
| `expose`   | EndpointExpose | OPTIONAL     | External access configuration.                                             |

#### EndpointExpose

| Field     | Type    | Required | Description                                                    |
| --------- | ------- | -------- | -------------------------------------------------------------- |
| `enabled` | boolean | OPTIONAL | Whether to create external access (ingress). Default: `false`. |
| `domain`  | string  | OPTIONAL | Domain for external access.                                    |

---

## 13. Validation Rules

Implementations MUST enforce the following rules:

1. `spec` MUST be `deployment/v1`.
2. `source.name`, `source.build`, and `source.registry` MUST be non-empty strings.
3. If `target.runtime` is present (non-empty), it MUST be `kubernetes`. An absent or empty value is valid — `runtime` is not currently enforced as required.
4. `agent.image` MUST be a non-empty string and `agent.endpoints` MUST contain at least one entry.
5. For each entry in `models`: `image` MUST be a non-empty string, `endpoints` MUST contain at least one entry, and each endpoint `port` MUST be a positive integer.
6. For each entry in `knowledge` where `binding` is set: no other field is validated — a bound entry's container config is fully unused, and correctness is checked against the store record at deploy time instead. For each entry where `binding` is absent: the same rules as `models` apply, plus rule 7.
7. For each entry in `knowledge`: when `persistent` is `true`, `storage` MUST be present.
8. For each entry in `integrations`: the same rules as `models` apply (`image` non-empty, at least one endpoint, positive ports).
9. When `endpoint.protocol` is provided, it MUST be one of `http`, `grpc`, or `tcp`.
10. For each entry in `ingestion`: `image` MUST be a non-empty string, `trigger` MUST be present, and `trigger.type` MUST be one of `schedule`, `startup`, `manual`, `webhook`.
11. When `trigger.type` is `schedule`, `trigger.schedule` MUST be a non-empty string containing a valid cron expression.
12. When `trigger.type` is NOT `schedule`, `trigger.schedule` MUST NOT be present.
13. When `trigger.type` is `webhook`, `endpoints` MUST contain at least one entry.
14. When `interfaces` is present and configures a messaging sidecar (`adapters` non-empty or `image` set): `adapters` MUST be a non-empty array and `image` MUST be a non-empty string. An `interfaces` block that sets only `auth.custom` (no sidecar) is exempt from this rule.
15. When `interfaces.adapters` includes `web`: `interfaces.endpoints` MUST contain an exposed `http` endpoint (or an endpoint with `expose.enabled: true`).
16. For each entry in `variables` where `optional` is `false` or absent: exactly one of `value` or `ref` MUST be present and non-empty.
17. `variables.*.targets` MUST be a non-empty array. Each element MUST be `agent`, `ingestion`, `ingestion.<name>` where `<name>` is a key in `ingestion`, or `interface.<adapter>` where `<adapter>` is a name listed in `interfaces.adapters`.
18. All `${}` references in `agent.environment` and `interfaces.environment` MUST resolve to a declared component, variable, or source attribute.
19. There MUST NOT be duplicate ports within one component's own `endpoints` map. Port uniqueness is **not** enforced across components or across the deployment as a whole — two different components may use the same port.
20. When `gpu.runtime` is provided, it MUST be one of `cuda` or `rocm`.
21. When `storage.access_mode` is provided, it MUST be one of `ReadWriteOnce` or `ReadWriteMany`.
22. When `update.strategy` is provided, it MUST be one of `rolling` or `recreate`.
23. When `agent.distributed` is `false` or absent, `agent.replicas` MUST NOT exceed `1`.
24. Each grant in `interfaces.auth.*.grants` MUST set exactly one of `org`, `user_id`, or `anyone`. Duplicate grants for the same adapter and subject are rejected.
25. When `interfaces.auth.web.public` is `true`: `interfaces.auth.web.grants` MUST include at least one `anyone` grant, and MUST NOT include any `org`/`user_id` grant — a public (no-OIDC) web surface has no identity to check those against. This rule does not apply to `interfaces.auth.custom`, whose grants aren't enforced by the platform at all (see Section 8).

---

## Appendix A: API Surface (Non-Normative)

### Deploy Endpoint

```
POST /api/v1/deploy
Content-Type: application/yaml or application/json
Body: deployment/v1 spec
```

The server validates, resolves `${}` references to concrete DNS/ports, stores the spec (secret variable values stripped), translates to infrastructure manifests, and applies.

### Storage

The deployment spec (secret variable values stripped) is stored alongside the deployment record. This provides an audit trail, enables redeploy, diff, and rollback.

---

## Appendix B: Resource Defaults (Non-Normative)

Default resource values applied by the server when omitted:

| Component Type                          | CPU Request | Memory Request | CPU Limit | Memory Limit |
| --------------------------------------- | ----------- | -------------- | --------- | ------------ |
| Standard (agent, non-GPU models, integrations) | `100m` | `1Gi`          | `100m`    | `1Gi`        |
| GPU workloads (models with `gpu`)       | `2`         | `8Gi`          | `4`       | `16Gi`       |
| Messaging sidecars                      | `100m`      | `256Mi`        | `100m`    | `256Mi`      |
| Collector                               | `50m`       | `128Mi`        | `50m`     | `128Mi`      |

Only the GPU tier's limits exceed its requests (2x on both CPU and memory). Standard, Messaging, and Collector all set CPU and memory limits equal to their requests — no burst headroom. This is a real, current characteristic of the defaults, not an omission in this table.

---

## Appendix C: Complete Example

```yaml
spec: deployment/v1

source:
  name: engineering-assistant
  build: "7"
  registry: registry.astro.dev/acme

target:
  runtime: kubernetes
  display_name: engineering-assistant-prod

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
    QDRANT_HOST: "${knowledge.docs.host}"
    QDRANT_PORT: "${knowledge.docs.http.port}"
    QDRANT_URL: "${knowledge.docs.http.url}"
    ANTHROPIC_API_KEY: "${variables.ANTHROPIC_API_KEY}"
    ASTRO_AGENT_NAME: "${source.name}"
    ASTRO_AGENT_BUILD: "${source.build}"
  update:
    strategy: rolling
    max_unavailable: "25%"
    max_surge: "25%"

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
      access_mode: ReadWriteOnce
    healthcheck:
      path: /healthz
    update:
      strategy: recreate

variables:
  ANTHROPIC_API_KEY:
    value: sk-ant-...
    targets: [agent]
    secret: true

observability:
  enabled: true
  provider: langfuse
```
