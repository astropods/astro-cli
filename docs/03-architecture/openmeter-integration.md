# OpenMeter Integration

This document describes how Astro integrates with OpenMeter for usage metering, billing, and entitlement enforcement.

## Overview

OpenMeter tracks resource consumption across the platform. Each Astro account maps 1:1 to an OpenMeter customer, with `account.id` as the universal subject key for event attribution. OpenMeter is internal infrastructure accessed via `OPENMETER_URL` with no authentication.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         USAGE METERING ARCHITECTURE                         │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────────────┐        ┌──────────────────────┐
  │   Request Handlers   │        │   Background Jobs    │
  │                      │        │   (every 5 min)      │
  │  agents.go           │        │                      │
  │  deploy.go           │        │  sync.go             │
  │  members.go          │        │  - compute heartbeat │
  │  accounts.go         │        │  - gauge snapshots   │
  └──────────┬───────────┘        └──────────┬───────────┘
             │ inline events                  │ heartbeat events
             ▼                                ▼
  ┌──────────────────────────────────────────────────────┐
  │              internal/openmeter/client.go             │
  │                                                      │
  │  CreateCustomer()    IngestEvents()                  │
  │  GetEntitlement()    CreateSubscription()            │
  └──────────────────────────┬───────────────────────────┘
                             │ CloudEvents (async, fire-and-forget)
                             ▼
                   ┌──────────────────┐
                   │    OpenMeter     │
                   │  (internal API)  │
                   └──────────────────┘
```

## Code Structure

```
apps/astro-server/internal/openmeter/
  client.go       -- Typed HTTP client wrapping OpenMeter REST API
  events.go       -- CloudEvent builder helpers per meter type
  middleware.go   -- Gin middleware for entitlement checks
  sync.go         -- Background jobs (compute heartbeats, gauge snapshots)
```

The client is initialized with `OPENMETER_URL` and injected into handlers via dependency injection (same pattern as other services in `main.go`).

## Customer Management

### Account-to-Customer Mapping

| Astro                     | OpenMeter                    |
| ------------------------- | ---------------------------- |
| `accounts.id` (UUID)      | Customer `key` + subject key |
| `accounts.name`           | Customer `name`              |
| `accounts.type`           | Customer `metadata.type`     |
| Owner email (from WorkOS) | Customer `primaryEmail`      |

The `accounts` table stores the OpenMeter customer ID in `openmeter_customer_id`.

### Customer Lifecycle

When an account is created (`handlers/accounts.go` CreateAccount):

1. Insert account into Postgres
2. Call OpenMeter `POST /api/v1/customers`:
   ```json
   {
     "name": "<account.name>",
     "key": "<account.id>",
     "usageAttribution": { "subjectKeys": ["<account.id>"] },
     "primaryEmail": "<owner_email>",
     "metadata": { "type": "<personal|organization>" }
   }
   ```
3. Store returned `id` as `accounts.openmeter_customer_id`
4. Auto-subscribe the customer to the active plan

Customer creation is **non-blocking** -- failures are logged and retried via background reconciliation so they never block account creation.

### Backfill

One-time migration script iterates all existing accounts and creates corresponding OpenMeter customers. Runs as a CLI command or background job.

## Meters

Nine meters track the primary billable dimensions of the platform.

### Deployment Meters

| Meter | Slug | Event Type | Aggregation | Emission |
|-------|------|------------|-------------|----------|
| Compute | `compute` | `compute_usage` | `SUM` on `$.compute_unit_hours` | Background heartbeat (5 min) |
| Active Agents | `agents` | `active_agents` | `LATEST` on `$.count` | Background snapshot (5 min) |
| Agent Builds | `agent_builds` | `agent_build` | `COUNT` | Inline in RegisterAgent handler |
| Active Deployments | `agent_deployments` | `active_deployments` | `LATEST` on `$.count` | Background snapshot (5 min) |
| Account Members | `members` | `active_members` | `LATEST` on `$.count` | Inline on member change + background reconciliation |

### Knowledge Store Meters

| Meter | Slug | Event Type | Aggregation | GroupBy | Emission |
|-------|------|------------|-------------|---------|----------|
| Active Knowledge Stores | `knowledge_stores` | `active_knowledge_stores` | `LATEST` on `$.count` | — | Inline on create/connect/delete + background snapshot (5 min) |
| Knowledge Storage | `knowledge_storage` | `knowledge_storage_provisioned` | `LATEST` on `$.storage_gb` | `$.store_name`, `$.provider` | Inline on create/delete + background snapshot (5 min) |
| Knowledge Compute | `knowledge_compute` | `knowledge_compute_usage` | `SUM` on `$.compute_unit_hours` | `$.store_name`, `$.provider` | Background heartbeat (5 min) |
| Knowledge Endpoints | `knowledge_endpoints` | `active_knowledge_endpoints` | `LATEST` on `$.count` | — | Inline on connect-with-PrivateLink + background snapshot (5 min) |

Knowledge store meters only track **managed** stores for storage and compute — external stores don't consume platform infrastructure. The store count meter includes both managed and external stores. This metering model is infrastructure-agnostic: the same meters apply whether stores run as K8s StatefulSets (current) or managed database instances like RDS (future), only the heartbeat's CU calculation source changes.

### Compute Units

Compute is measured in **compute-unit-hours** (1 CU = 1 vCPU + 2GB RAM per hour, analogous to Lambda's GB-seconds). The background job runs every 5 minutes and for each active deployment:

1. Reads CPU and memory requests from the deployment spec
2. Normalizes to compute units: `CU = max(cpu_cores, memory_gb / 2)`
3. Emits `$.compute_unit_hours = CU * (interval_minutes / 60)`

Source: Polls `deployments` table (`undeployed_at IS NULL`) + K8s pod resource requests. GroupBy: `$.agent_name`, `$.namespace`, `$.component`.

### Knowledge Compute Units

Knowledge stores run as independent StatefulSets with their own CPU/memory resource requests. The same CU formula applies: `CU = max(cpu_cores, memory_gb / 2)`. The heartbeat queries managed stores with `status = 'ready'` and looks up per-provider resource requests from `knowledgeProviderResources()`.

Per-provider CU per heartbeat (at default resource requests):

| Provider | CPU Request | Memory Request | CU | CU-hours/month (24/7) |
|----------|-----------|--------------|-----|----------------------|
| postgres | 250m | 256Mi | 0.25 | ~180 |
| redis | 50m | 64Mi | 0.05 | ~36 |
| qdrant | 250m | 512Mi | 0.25 | ~180 |
| neo4j | 500m | 512Mi | 0.50 | ~360 |

If the backing infrastructure moves from K8s StatefulSets to managed services (e.g. RDS), the CU calculation source changes (instance class specs instead of pod resource requests) but the meter, event schema, and entitlement logic remain the same.

### Knowledge Storage

Storage is metered as **provisioned GB**, not actual usage. This matches the underlying cost model — EBS volumes (backing PVCs) bill on allocated size. Events are emitted per store with `store_name` and `provider` in the payload for per-store granularity. The K8s quantity in the `storage` column (e.g. "10Gi", "500Mi") is parsed to GB at emission time.

### Gauge Meters

Agents, deployments, members, knowledge stores, and knowledge endpoints are **gauge** meters using `LATEST` aggregation. Background jobs snapshot the current count every 5 minutes. After a monthly billing reset, heartbeats immediately re-report the actual state so enforcement reflects reality within minutes.

### Meter Definitions (Production)

Current meter definitions in OpenMeter, created via `POST /api/v1/meters`:

```json
{
  "slug": "compute",
  "name": "Compute Consumed",
  "description": "Compute-unit-hours consumed by active deployments, per container",
  "eventType": "compute_usage",
  "aggregation": "SUM",
  "valueProperty": "$.compute_unit_hours",
  "groupBy": {
    "agent_name": "$.agent_name",
    "namespace": "$.namespace",
    "component": "$.component"
  }
}
```

```json
{
  "slug": "agents",
  "name": "Active Agents",
  "description": "Number of distinct agents registered per account",
  "eventType": "active_agents",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}
```

```json
{
  "slug": "agent_deployments",
  "name": "Active Deployments",
  "description": "Number of currently active agent deployments per account",
  "eventType": "active_deployments",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}
```

```json
{
  "slug": "members",
  "name": "Account Members",
  "description": "Number of members in each account",
  "eventType": "active_members",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}
```

```json
{
  "slug": "agent_builds",
  "name": "Agent Builds",
  "description": "Number of agent builds pushed to the registry",
  "eventType": "agent_build",
  "aggregation": "COUNT",
  "groupBy": {
    "agent_name": "$.agent_name"
  }
}
```

```json
{
  "slug": "knowledge_stores",
  "name": "Active Knowledge Stores",
  "description": "Total number of knowledge stores (managed + external) per account",
  "eventType": "active_knowledge_stores",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}
```

```json
{
  "slug": "knowledge_storage",
  "name": "Knowledge Storage Provisioned",
  "description": "Provisioned storage in GB per managed knowledge store",
  "eventType": "knowledge_storage_provisioned",
  "aggregation": "LATEST",
  "valueProperty": "$.storage_gb",
  "groupBy": {
    "store_name": "$.store_name",
    "provider": "$.provider"
  }
}
```

```json
{
  "slug": "knowledge_compute",
  "name": "Knowledge Compute",
  "description": "Compute-unit-hours consumed by managed knowledge store StatefulSets",
  "eventType": "knowledge_compute_usage",
  "aggregation": "SUM",
  "valueProperty": "$.compute_unit_hours",
  "groupBy": {
    "store_name": "$.store_name",
    "provider": "$.provider"
  }
}
```

```json
{
  "slug": "knowledge_endpoints",
  "name": "Active Knowledge Endpoints",
  "description": "Number of active PrivateLink VPC endpoints per account",
  "eventType": "active_knowledge_endpoints",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}
```

## Event Ingestion

All events use CloudEvents format with `subject` = `account.id`. Event ingestion is **async fire-and-forget** via buffered channel so it never blocks request handling. Failed events are logged and can be retried.

### `agent_build` (inline -- RegisterAgent handler)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "agent_build",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "agent_name": "my-agent"
  }
}
```

### `compute_usage` (heartbeat -- every 5 min, one per container)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "compute_usage",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "compute_unit_hours": 0.0833,
    "agent_name": "my-agent",
    "namespace": "astro-abc123",
    "component": "model/llm",
    "cpu": "2",
    "memory": "8Gi",
    "replicas": 1
  }
}
```

### `active_deployments` / `active_agents` (heartbeat -- every 5 min, one per account)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "active_deployments",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "count": 3
  }
}
```

### `active_members` (inline on member change + heartbeat reconciliation)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "active_members",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "count": 4
  }
}
```

### `active_knowledge_stores` (inline on create/connect/delete + heartbeat)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "active_knowledge_stores",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "count": 4
  }
}
```

### `knowledge_storage_provisioned` (inline on create/delete + heartbeat, one per managed store)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "knowledge_storage_provisioned",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "storage_gb": 10.0,
    "store_name": "my-pgvector",
    "provider": "postgres"
  }
}
```

### `knowledge_compute_usage` (heartbeat -- every 5 min, one per managed+ready store)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "knowledge_compute_usage",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "compute_unit_hours": 0.0208,
    "store_name": "my-pgvector",
    "provider": "postgres",
    "cpu": "250m",
    "memory": "256Mi"
  }
}
```

### `active_knowledge_endpoints` (inline on connect-with-PrivateLink + heartbeat)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "active_knowledge_endpoints",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "count": 1
  }
}
```

## Plans and Entitlements

### Features

One feature per meter, used as the entitlement key for limit checks. Current feature definitions in OpenMeter, created via `POST /api/v1/features`:

```json
{ "key": "compute", "name": "Compute", "meterSlug": "compute", "metadata": { "unit": "CU-hours" } }
```
```json
{ "key": "agents", "name": "Agents", "meterSlug": "agents", "metadata": { "unit": "agents" } }
```
```json
{ "key": "agent_builds", "name": "Agent Builds", "meterSlug": "agent_builds", "metadata": { "unit": "builds" } }
```
```json
{ "key": "agent_deployments", "name": "Deployments", "meterSlug": "agent_deployments", "metadata": { "unit": "deployments" } }
```
```json
{ "key": "members", "name": "Members", "meterSlug": "members", "metadata": { "unit": "members" } }
```
```json
{ "key": "knowledge_stores", "name": "Knowledge Stores", "meterSlug": "knowledge_stores", "metadata": { "unit": "stores" } }
```
```json
{ "key": "knowledge_storage", "name": "Knowledge Storage", "meterSlug": "knowledge_storage", "metadata": { "unit": "GB" } }
```
```json
{ "key": "knowledge_compute", "name": "Knowledge Compute", "meterSlug": "knowledge_compute", "metadata": { "unit": "CU-hours" } }
```
```json
{ "key": "knowledge_endpoints", "name": "Knowledge Endpoints", "meterSlug": "knowledge_endpoints", "metadata": { "unit": "endpoints" } }
```

### Private Beta Plan

Single plan (`private_beta`), free, with hard limits. All accounts are auto-subscribed on creation. Uses metered entitlements with `issueAfterReset` so grants are auto-provisioned and `hasAccess` enforcement works automatically.

| Feature | Limit | Reset | Type |
|---------|-------|-------|------|
| `compute` | 100 CU-hours/mo | Monthly, cumulative | Counter |
| `agent_builds` | 50 builds/mo | Monthly, cumulative | Counter |
| `agents` | 5 | Monthly, gauge re-reports | Gauge |
| `agent_deployments` | 10 | Monthly, gauge re-reports | Gauge |
| `members` | 5 | Monthly, gauge re-reports | Gauge |
| `knowledge_stores` | 5 | Monthly, gauge re-reports | Gauge |
| `knowledge_storage` | 50 GB | Monthly, gauge re-reports | Gauge |
| `knowledge_compute` | 50 CU-hours/mo | Monthly, cumulative | Counter |
| `knowledge_endpoints` | 2 | Monthly, gauge re-reports | Gauge |

Current plan definition in OpenMeter, created via `POST /api/v1/plans`:

```json
{
  "key": "private_beta",
  "name": "Private Beta",
  "description": "Free tier for private beta users with hard limits on all resources.",
  "currency": "USD",
  "billingCadence": "P1M",
  "phases": [
    {
      "key": "beta",
      "name": "Private Beta",
      "description": "Single phase — free access with hard-capped entitlements.",
      "duration": null,
      "rateCards": [
        {
          "type": "usage_based",
          "key": "compute",
          "name": "Compute",
          "description": "Compute-unit-hours consumed by active deployments (1 CU = 1 vCPU + 2 GB RAM per hour).",
          "featureKey": "compute",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 100
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "agents",
          "name": "Agents",
          "description": "Number of distinct agents registered in the account.",
          "featureKey": "agents",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 5
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "agent_builds",
          "name": "Agent Builds",
          "description": "Number of agent builds pushed to the registry per billing period.",
          "featureKey": "agent_builds",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 50
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "agent_deployments",
          "name": "Deployments",
          "description": "Number of concurrently active agent deployments.",
          "featureKey": "agent_deployments",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 10
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "members",
          "name": "Members",
          "description": "Number of members in the account.",
          "featureKey": "members",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 5
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "knowledge_stores",
          "name": "Knowledge Stores",
          "description": "Number of knowledge stores (managed + external) per account.",
          "featureKey": "knowledge_stores",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 5
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "knowledge_storage",
          "name": "Knowledge Storage",
          "description": "Total provisioned storage (GB) across managed knowledge stores.",
          "featureKey": "knowledge_storage",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 50
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "knowledge_compute",
          "name": "Knowledge Compute",
          "description": "Compute-unit-hours consumed by managed knowledge store infrastructure (1 CU = 1 vCPU + 2 GB RAM per hour).",
          "featureKey": "knowledge_compute",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 50
          },
          "price": null
        },
        {
          "type": "usage_based",
          "key": "knowledge_endpoints",
          "name": "Knowledge Endpoints",
          "description": "Number of active PrivateLink VPC endpoints for external knowledge stores.",
          "featureKey": "knowledge_endpoints",
          "billingCadence": "P1M",
          "entitlementTemplate": {
            "type": "metered",
            "isSoftLimit": false,
            "issueAfterReset": 2
          },
          "price": null
        }
      ]
    }
  ]
}
```

### Subscription Lifecycle

On account creation, after the OpenMeter customer is created:

```
POST /api/v1/subscriptions
{
  "name": "<account.name> - Private Beta",
  "customerId": "<openmeter_customer_id>",
  "plan": { "key": "private_beta", "version": 1 },
  "activeFrom": "<now>",
  "billingCadence": "P1M",
  "billingAnchor": "<now>"
}
```

## Entitlement Enforcement

Before resource-consuming operations, entitlements are checked synchronously:

| Operation | Entitlements Checked |
|-----------|---------------------|
| Deploy agent | `agent_deployments`, `compute` |
| Register agent | `agents` |
| Add member | `members` |
| Create knowledge store | `knowledge_stores`, `knowledge_storage`, `knowledge_compute` |
| Connect knowledge store | `knowledge_stores` |
| Connect knowledge store (PrivateLink) | `knowledge_stores`, `knowledge_endpoints` |

Entitlement responses are cached with a short TTL (~30s) to avoid per-request latency. When limits are exceeded, the API returns **402** with entitlement details so the client can show upgrade prompts. Reads remain allowed; only writes/deploys are blocked.

## Design Decisions

- **Per-account metering only.** Single subject key per account, no per-user breakdown.
- **Eventually consistent customer creation.** OpenMeter failures never block account creation; reconciled via background job.
- **Degraded responses over hard 403s.** Reads allowed, writes blocked with 402 + entitlement details.
- **No auth to OpenMeter.** Internal service, accessed directly via `OPENMETER_URL`.
- **Auto-assign plan on account creation.** Plan selection UI deferred to a later phase.
- **No threshold notifications yet.** OpenMeter's notification system is not currently used.
