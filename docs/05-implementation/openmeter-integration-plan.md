# OpenMeter Integration Plan

## Overview

Integrate OpenMeter into astro-server for usage metering, billing, and entitlement enforcement. OpenMeter customers map 1:1 to Astro accounts, with `account.id` as the universal subject key for event attribution.

## Current State

- `OPENMETER_URL` env var exists; admin gRPC has a `ProxyOpenMeter` passthrough (`admingrpc/server.go:651`)
- No customer creation, event ingestion, or entitlement checking in the public API path
- No `openmeter_customer_id` stored in the database

---

## 1. Meters

Five core meters, mapped to the primary billable dimensions of the platform.

### 1a. `compute` -- Compute and Memory Consumed

Resource-weighted metric combining CPU and memory over time, measured in **compute-unit-hours** (1 CU = 1 vCPU + 2GB RAM per hour). Analogous to Lambda's GB-seconds.

- Event type: `compute_usage`
- Aggregation: `SUM` on `$.compute_unit_hours`
- GroupBy: `$.agent_name`, `$.namespace`
- Emission: Background job runs every 5 minutes. For each active deployment:
  1. Read CPU and memory requests from the deployment spec
  2. Normalize to compute units: `CU = max(cpu_cores, memory_gb / 2)`
  3. Emit `$.compute_unit_hours = CU * (interval_minutes / 60)`
- Source: Polls `deployments` table (`undeployed_at IS NULL`) + K8s pod resource requests

### 1b. `agents` -- Active Agents

Tracks how many distinct agents exist per account (gauge via periodic snapshot).

- Event type: `active_agents`
- Aggregation: `LATEST` on `$.count`
- GroupBy: *(none — account-level total)*
- Emission: Background job (Phase 3) counts distinct agents per account and emits the current count

### 1c. `agent_builds` -- Agent Builds

Tracks each build/version pushed to the registry.

- Event type: `agent_build`
- Aggregation: `COUNT`
- GroupBy: `$.agent_name`
- Emission: `handlers/agents.go` RegisterAgent handler, after successful version creation

### 1d. `agent_deployments` -- Active Deployments

Tracks how many agents are currently deployed (gauge via periodic snapshot).

- Event type: `active_deployments`
- Aggregation: `LATEST` on `$.count`
- GroupBy: *(none — account-level total)*
- Emission: Background job (Phase 3) queries `deployments WHERE status = 'active'` per account every 5 minutes and emits the current count

### 1e. `members` -- Account Members

Tracks how many members belong to each account (gauge via periodic snapshot).

- Event type: `active_members`
- Aggregation: `LATEST` on `$.count`
- GroupBy: *(none — account-level total)*
- Emission: Inline on member add/remove (`handlers/members.go`), plus background job (Phase 3) snapshots `account_members` count per account every 5 minutes as reconciliation

### Meter Creation Payloads

`POST /api/v1/meters` for each:

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

---

## 2. Customer Management

### Account-to-Customer Mapping

| Astro                     | OpenMeter                    |
| ------------------------- | ---------------------------- |
| `accounts.id` (UUID)      | Customer `key` + subject key |
| `accounts.name`           | Customer `name`              |
| `accounts.type`           | Customer `metadata.type`     |
| Owner email (from WorkOS) | Customer `primaryEmail`      |

### Schema Change

```sql
ALTER TABLE public.accounts ADD COLUMN openmeter_customer_id text;
```

### Customer Creation Flow

On `POST /api/v1/accounts` (in `handlers/accounts.go` CreateAccount), after inserting the account into Postgres:

1. Call OpenMeter `POST /api/v1/customers`:
   ```json
   {
     "name": "<account.name>",
     "key": "<account.id>",
     "usageAttribution": { "subjectKeys": ["<account.id>"] },
     "primaryEmail": "<owner_email>",
     "metadata": { "type": "<personal|organization>" }
   }
   ```
2. Store returned `id` as `accounts.openmeter_customer_id`
3. On failure: log error, do not block account creation (OpenMeter is non-critical path). Retry via background reconciliation.

### Backfill

One-time migration script that iterates all existing accounts and creates corresponding OpenMeter customers. Can run as a CLI command or background job.

---

## 3. Event Ingestion

All events use CloudEvents format with `subject` = `account.id`. Below are the exact payloads emitted by each source.

### `agent_build` (inline — RegisterAgent handler)

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

### `compute_usage` (heartbeat — every 5 min, one per container)

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

### `active_deployments` (heartbeat — every 5 min, one per account)

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

### `active_agents` (heartbeat — every 5 min, one per account)

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "active_agents",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": {
    "count": 5
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

Event ingestion is **async fire-and-forget** (buffered channel or goroutine) so it never blocks request handling. Failed events are logged and can be retried.

---

## 4. Plans and Subscriptions

### Features

One feature per meter, used as the entitlement key for limit checks.

`POST /api/v1/features` for each:

```json
{
  "key": "compute",
  "name": "Compute",
  "meterSlug": "compute",
  "metadata": { "unit": "CU-hours" }
}
```
```json
{
  "key": "agents",
  "name": "Agents",
  "meterSlug": "agents",
  "metadata": { "unit": "agents" }
}
```
```json
{
  "key": "agent_builds",
  "name": "Agent Builds",
  "meterSlug": "agent_builds",
  "metadata": { "unit": "builds" }
}
```
```json
{
  "key": "agent_deployments",
  "name": "Deployments",
  "meterSlug": "agent_deployments",
  "metadata": { "unit": "deployments" }
}
```
```json
{
  "key": "members",
  "name": "Members",
  "meterSlug": "members",
  "metadata": { "unit": "members" }
}
```

### Plan: Private Beta

Single plan, free, hard limits on everything. All accounts are auto-subscribed on creation.

`POST /api/v1/plans`

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
        }
      ]
    }
  ]
}
```

### Private Beta Limits

All features use metered entitlements with `issueAfterReset` so `hasAccess` enforcement
works automatically and grants are auto-provisioned on subscription. No manual grant
creation needed.

For gauge features (agents, deployments, members), the grant resets monthly but
heartbeats immediately re-report the current count, so enforcement reflects the
actual state within minutes of each period start.

| Feature             | Limit          | `issueAfterReset` | Notes |
| ------------------- | -------------- | ------------------ | ----- |
| `compute`           | 100 CU-hours   | 100                | Cumulative, resets monthly |
| `agent_builds`      | 50 builds      | 50                 | Cumulative, resets monthly |
| `agents`            | 5 agents       | 5                  | Gauge — heartbeat re-reports current count after reset |
| `agent_deployments` | 10 deployments | 10                 | Gauge — heartbeat re-reports current count after reset |
| `members`           | 5 members      | 5                  | Gauge — heartbeat re-reports current count after reset |

### Subscription Creation

Auto-subscribe on account creation (in `handlers/accounts.go` after OpenMeter customer is created):

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

---

## 5. Entitlement Enforcement

Before resource-consuming operations, check entitlements:

- **DeployAgent**: Check `agent_deployments` and `compute` entitlements
- **RegisterAgent**: Check `agent_registrations` entitlement
- **AddMember**: Check `members` entitlement

Entitlement checks are **sync** (block the request). Cache with short TTL (~30s) to avoid per-request latency.

---

## 6. New Code Structure

```
apps/astro-server/
  internal/
    openmeter/
      client.go         -- Typed HTTP client (customer CRUD, event ingestion, entitlement queries)
      events.go         -- CloudEvent builder helpers per meter type
      middleware.go      -- Gin middleware for entitlement checks
      sync.go           -- Background jobs (compute heartbeats, message count sync from Galileo)
```

### client.go

Wraps OpenMeter REST API. Methods:
- `CreateCustomer(ctx, account) (customerID, error)`
- `IngestEvents(ctx, []CloudEvent) error`
- `GetEntitlement(ctx, customerID, featureKey) (Entitlement, error)`
- `CreateSubscription(ctx, customerID, planKey) error`

Initialized with `OPENMETER_URL` (no auth). Passed into handlers via dependency injection (same pattern as other services in main.go).

### Background Jobs

- **Compute heartbeat**: Runs every 5 minutes. For each active deployment, computes CU-hours for the interval from CPU/memory requests and emits `compute_usage` events.

---

## 7. Implementation Order

| Phase       | Status  | Work                                                                                        | Files                                                                                  |
| ----------- | ------- | ------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| ~~Phase 1~~ | Done    | OpenMeter client + customer creation on account create + backfill migration                 | `internal/openmeter/client.go`, `handlers/accounts.go`, `schema.sql`, migration script |
| ~~Phase 2~~ | Done    | Inline meters: `agent_deployments` + `agent_registrations` event emission                   | `internal/openmeter/events.go`, `handlers/deploy.go`, `handlers/agents.go`             |
| ~~Phase 3~~ | Done    | Background meters: `compute` heartbeat                                                      | `internal/openmeter/sync.go`                                                           |
| **Phase 4** | Next    | `members` meter: inline emission on member add/remove + background heartbeat reconciliation | `internal/openmeter/events.go`, `handlers/members.go`, `internal/openmeter/sync.go`    |
| **Phase 5** | Planned | Plan/subscription management + entitlement enforcement middleware                           | `internal/openmeter/middleware.go`, plan definitions in OpenMeter                      |

---

## Open Questions

- [x] Should customer creation failure block account creation, or be eventually consistent? **Eventually consistent.** Log error, continue, reconcile via background job.
- [x] Do we need per-user metering within an organization, or only per-account? **Per-account only.** Single subject key per account, no per-user breakdown.
- [x] Where does plan selection UI live -- astro-client, astro-queen, or both? **Auto-assign default plan on account creation.** Plan selection UI in both astro-client and astro-queen later.
- [x] Should entitlement checks return degraded responses (e.g. read-only mode) or hard 403s? **Degraded responses.** Allow reads, block writes/deploys. Return 402 with entitlement details so the client can show upgrade prompts.
- [x] What is the API key auth mechanism for OpenMeter -- env var, secret, or service account? **No auth.** OpenMeter is internal/private, accessed directly via `OPENMETER_URL`.
- [x] Should the Galileo token sync be real-time (webhook from Galileo) or polled? **N/A.** Galileo is not used for metering at all.
- [x] Do we need OpenMeter's notification system for threshold alerts (75%, 100% usage)? **Not yet.** No notifications for now.
