# Deployment Spec Normalization

## Problem

`deployments.deployment_spec_json` stores the entire `AstroDeploymentSpec` as a JSON text blob. This makes it impossible to:

- Query individual component properties (e.g. "which deployments use GPU?")
- Detect drift between desired state (DB) and actual state (K8s)
- Enforce constraints at the DB level
- Build a reconciliation loop that operates on structured data

## Design: EKS-Resource-Oriented Schema

A deployment produces a set of K8s workloads (Deployments, StatefulSets, CronJobs, Jobs) each with associated Services, Ingresses, Secrets, ConfigMaps, and resource requirements. The DB stores the **desired state** of these K8s objects so a reconciler can diff against the cluster.

### Spec → K8s Resource Mapping

Each component in the deployment spec maps to exactly one K8s workload:

| Component | `workload_type` | Condition                               |
| --------- | --------------- | --------------------------------------- |
| Agent     | `deployment`    | Always                                  |
| Model     | `statefulset`   | `persistent=true`                       |
| Model     | `deployment`    | `persistent=false`                      |
| Knowledge | `statefulset`   | `persistent=true`                       |
| Knowledge | `deployment`    | `persistent=false`                      |
| Tool      | `deployment`    | Always                                  |
| Ingestion | `cronjob`       | `trigger_type=schedule`                 |
| Ingestion | `job`           | `trigger_type=startup`                  |
| Ingestion | `deployment`    | `trigger_type=webhook`                  |
| Ingestion | (none)          | `trigger_type=manual` (annotation only) |
| Messaging | `deployment`    | `interfaces.adapters` non-empty         |
| Collector | `deployment`    | `observability.enabled=true`            |

### Relationships

```
deployments
 ├── deployment_workloads (1:N)
 │    ├── deployment_services (1:N)
 │    │    └── deployment_ingresses (1:1)
 │    ├── deployment_volumes (1:N, statefulsets only)
 │    └── deployment_env_vars (1:N)
 └── deployment_variables (1:N)
```

## Tables

### `deployment_workloads`

One row per K8s workload. This is the core table — one workload = one reconciliation unit.

| Column                   | Type                         | Notes                                                                        |
| ------------------------ | ---------------------------- | ---------------------------------------------------------------------------- |
| `id`                     | serial PK                    |                                                                              |
| `deployment_id`          | varchar(11) FK → deployments |                                                                              |
| `name`                   | varchar                      | K8s resource name (e.g. `myagent-agent`, `myagent-model-gpt4`)               |
| `component_kind`         | varchar                      | `agent`, `model`, `knowledge`, `tool`, `ingestion`, `messaging`, `collector` |
| `component_key`          | varchar                      | Map key from spec (empty for agent/messaging/collector)                      |
| `workload_type`          | varchar                      | `deployment`, `statefulset`, `cronjob`, `job`                                |
| `image`                  | varchar                      | Container image                                                              |
| `replicas`               | int                          | 0 for jobs/cronjobs                                                          |
| `cpu_request`            | varchar                      | e.g. `100m`                                                                  |
| `memory_request`         | varchar                      | e.g. `256Mi`                                                                 |
| `cpu_limit`              | varchar                      |                                                                              |
| `memory_limit`           | varchar                      |                                                                              |
| `gpu_vram`               | varchar                      | nullable                                                                     |
| `gpu_runtime`            | varchar                      | nullable (`cuda`/`rocm`)                                                     |
| `gpu_count`              | int                          | nullable                                                                     |
| `update_strategy`        | varchar                      | `rolling`/`recreate`, null for statefulsets/jobs                             |
| `update_max_unavailable` | varchar                      | nullable                                                                     |
| `update_max_surge`       | varchar                      | nullable                                                                     |
| `healthcheck_path`       | varchar                      | nullable                                                                     |
| `healthcheck_port`       | int                          | nullable                                                                     |
| `healthcheck_interval`   | varchar                      | nullable                                                                     |
| `healthcheck_timeout`    | varchar                      | nullable                                                                     |
| `healthcheck_retries`    | int                          | nullable                                                                     |
| `healthcheck_test`       | text                         | nullable (exec probe command)                                                |
| `trigger_type`           | varchar                      | nullable, ingestion only (`schedule`/`startup`/`webhook`/`manual`)           |
| `trigger_schedule`       | varchar                      | nullable, cron expression                                                    |
| `persistent`             | boolean                      | default false — determines deployment vs statefulset                         |
| `distributed`            | boolean                      | default false — agent only                                                   |

Unique: `(deployment_id, name)`.

A single flat table with `component_kind` + `workload_type` is simpler than separate `deployment_models`, `deployment_tools`, etc. The fields are 90% identical across component types; the few component-specific columns (`trigger_type`, `persistent`, `distributed`) are nullable.

### `deployment_services`

One row per K8s Service.

| Column        | Type                          | Notes               |
| ------------- | ----------------------------- | ------------------- |
| `id`          | serial PK                     |                     |
| `workload_id` | int FK → deployment_workloads |                     |
| `name`        | varchar                       | K8s service name    |
| `port`        | int                           | Service port        |
| `target_port` | int                           | Container port      |
| `protocol`    | varchar                       | `http`/`grpc`/`tcp` |

Unique: `(workload_id, name)`.

### `deployment_ingresses`

One row per K8s Ingress. Services and Ingresses can drift independently (someone deletes a Service, ALB gets removed) — separate tables let us detect that.

| Column        | Type                         | Notes                                                |
| ------------- | ---------------------------- | ---------------------------------------------------- |
| `id`          | serial PK                    |                                                      |
| `service_id`  | int FK → deployment_services |                                                      |
| `hostname`    | varchar                      | Generated hostname (e.g. `myagent-abc123.astro.dev`) |
| `path`        | varchar                      | default `/`                                          |
| `tls_enabled` | boolean                      | default true                                         |

Unique: `(service_id)`.

### `deployment_volumes`

PVC specs for StatefulSets. Stored explicitly because volumes are immutable in StatefulSets — a spec change here requires a StatefulSet recreate.

| Column          | Type                          | Notes                           |
| --------------- | ----------------------------- | ------------------------------- |
| `id`            | serial PK                     |                                 |
| `workload_id`   | int FK → deployment_workloads |                                 |
| `mount_path`    | varchar                       | e.g. `/data`, `/mnt/models`     |
| `size`          | varchar                       | e.g. `10Gi`                     |
| `storage_class` | varchar                       | nullable                        |
| `access_mode`   | varchar                       | `ReadWriteOnce`/`ReadWriteMany` |

Unique: `(workload_id, mount_path)`.

### `deployment_env_vars`

Resolved environment per workload. Env vars are the #1 drift surface — config changes, secret rotations, variable updates all manifest as env var changes. Having them queryable means we can diff without parsing JSON.

| Column        | Type                          | Notes                              |
| ------------- | ----------------------------- | ---------------------------------- |
| `workload_id` | int FK → deployment_workloads |                                    |
| `key`         | varchar                       | Env var name                       |
| `value`       | text                          | Resolved value (encrypted if `source='secret'`) |
| `source`      | varchar                       | `configmap`, `secret`, `direct`                 |
| `nonce`        | bytea                         | nullable, AES-256-GCM nonce (secret values only) |

PK: `(workload_id, key)`.

### `deployment_variables`

Top-level spec variables (for template/UI use, not K8s-facing).

| Column          | Type           | Notes                       |
| --------------- | -------------- | --------------------------- |
| `deployment_id` | varchar(11) FK |                             |
| `name`          | varchar        | Variable name               |
| `value`         | text           | Encrypted if `secret=true`  |
| `secret`        | boolean        |                             |
| `optional`      | boolean        |                             |
| `targets`       | text[]         | Which components consume it |
| `nonce`          | bytea          | nullable, AES-256-GCM nonce (secret values only) |

PK: `(deployment_id, name)`.

## Secret Encryption (AWS KMS Envelope Encryption)

Secret values are stored encrypted at rest using AES-256-GCM with AWS KMS envelope encryption. This replaces the current behavior of stripping secret values before storage.

### How It Works

**On deploy (encrypt):**

1. Server calls KMS `GenerateDataKey` with the configured KMS key ARN — KMS returns a plaintext data key + an encrypted copy of that key
2. Server uses the plaintext data key to AES-256-GCM encrypt each secret value locally, generating a unique nonce per value
3. Server stores the **encrypted values** (with per-value `nonce`) and the **encrypted data key** (on the `deployments` row) in Postgres
4. Server discards the plaintext data key from memory

**On read (decrypt):**

1. Server reads the `encrypted_data_key` from the `deployments` row
2. Server calls KMS `Decrypt` — KMS returns the plaintext data key
3. Server uses the plaintext data key + per-value `nonce` to decrypt secret values locally
4. Discards plaintext data key after use

### Why Envelope Encryption

- KMS has a 4KB payload limit — can't encrypt large values directly
- KMS API calls are rate-limited and add latency — one `GenerateDataKey` call per deployment is better than one call per secret
- The actual AES-256-GCM crypto happens locally, KMS only protects the key
- Audit trail via CloudTrail for all key usage

### Schema Changes

New columns on `deployments`:

| Column               | Type    | Notes                                      |
| -------------------- | ------- | ------------------------------------------ |
| `encrypted_data_key` | bytea   | nullable, KMS-encrypted data key           |
| `kms_key_arn`        | varchar | nullable, KMS key ARN used for this deploy |

Per-value nonces stored on `deployment_variables.nonce` and `deployment_env_vars.nonce` (see table definitions above).

### Configuration

Server env vars:

- `KMS_KEY_ARN` — ARN of the KMS key used for envelope encryption (required for new deployments)
- When unset, secrets are stored empty (current stripped behavior) as a fallback for local dev

## KMS Infrastructure Setup

Instructions for the infra agent to provision the KMS resources needed for deployment secret encryption.

### What to Create

**1. KMS Key**

Create a symmetric KMS key for envelope encryption of deployment secrets. Enable automatic annual key rotation. Set a 30-day deletion window. Alias: `alias/astro-deployment-secrets`.

**2. IAM Policy**

Create an IAM policy granting only two KMS actions on the key:
- `kms:GenerateDataKey` — called once per deployment to generate a data encryption key
- `kms:Decrypt` — called when reading secrets back to decrypt the data key

No other KMS permissions are needed. No `kms:Encrypt` (we use `GenerateDataKey` which returns both plaintext and ciphertext). No admin permissions.

**3. IRSA Attachment**

Attach the policy to the existing IAM role backing the `astro-server` Kubernetes service account (IRSA). The server pods need to call KMS via the AWS SDK using the service account's assumed role — no static credentials.

### What to Output

After provisioning, the server needs one value injected as an environment variable:

```
KMS_KEY_ARN=arn:aws:kms:{region}:{account}:key/{key-id}
```

Add this to however `astro-server` env vars are currently managed (Helm values, ConfigMap, etc.). This is the only configuration the application needs — the AWS SDK picks up credentials automatically via IRSA.

### Constraints

- One key for all deployments (not per-tenant) — tenant isolation is handled at the application layer via per-deployment data keys
- The key must be in the same region as the EKS cluster
- No key policy customization needed beyond the default (account root has admin, IAM policies control access)

## Migration Strategy

- **Keep `deployment_spec_json`** on the `deployments` table as an archive/fallback column. No rename, no removal. New tables become the source of truth going forward.
- **Dual-write**: On deploy, write both the JSON blob (backward compat) and normalized rows in the same transaction.
- **Read path**: New code reads from normalized tables. Falls back to JSON parsing for old deployments that predate the migration.
- **Backfill**: One-time migration script parses existing `deployment_spec_json` rows and populates the new tables.

## Implementation Phases

### Phase 1 — Schema + Dual Write

- Add tables to `schema.sql`
- New `deploymentstore` methods to save/query normalized data
- `handlers/deploy.go` writes both JSON blob and normalized rows in the same transaction
- Read path still uses JSON (existing behavior preserved)

### Phase 2 — Read from Normalized Tables

- API responses assembled from table queries
- Admin gRPC reads structured data
- OpenMeter heartbeat queries workloads table directly

### Phase 3 — Reconciler

- Periodic loop: for each active deployment, query desired state from DB, compare to K8s API, report/fix drift
