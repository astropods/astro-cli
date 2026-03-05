# OpenMeter Integration Plan

## Overview

Integrate OpenMeter into astro-server for usage metering, billing, and entitlement enforcement. OpenMeter customers map 1:1 to Astro accounts, with `account.id` as the universal subject key for event attribution.

## Current State

- `OPENMETER_URL` env var exists; admin gRPC has a `ProxyOpenMeter` passthrough (`admingrpc/server.go:651`)
- No customer creation, event ingestion, or entitlement checking in the public API path
- No `openmeter_customer_id` stored in the database

---

## 1. Meters

Four core meters, mapped to the primary billable dimensions of the platform.

### 1a. `messages` -- Messages Handled

Tracks the number of messages processed by deployed agents. The messaging container is Astro-controlled infrastructure (not user code), so it emits events directly to OpenMeter.

- Event type: `agent_message`
- Aggregation: `COUNT`
- GroupBy: `$.agent_name`, `$.namespace`
- Emission: The messaging container emits CloudEvents directly to OpenMeter on each message handled. Requires the container to know the `OPENMETER_URL`, API key, and the account's subject key (injected as env vars at deployment time).

### 1b. `compute` -- Compute and Memory Consumed

Resource-weighted metric combining CPU and memory over time, measured in **compute-unit-hours** (1 CU = 1 vCPU + 2GB RAM per hour). Analogous to Lambda's GB-seconds.

- Event type: `compute_usage`
- Aggregation: `SUM` on `$.compute_unit_hours`
- GroupBy: `$.agent_name`, `$.namespace`
- Emission: Background job runs every 5 minutes. For each active deployment:
  1. Read CPU and memory requests from the deployment spec
  2. Normalize to compute units: `CU = max(cpu_cores, memory_gb / 2)`
  3. Emit `$.compute_unit_hours = CU * (interval_minutes / 60)`
- Source: Polls `deployments` table (`undeployed_at IS NULL`) + K8s pod resource requests

### 1c. `agent_registrations` -- Agents Registered

Tracks how many agent builds/versions are registered (pushed to the registry).

- Event type: `agent_register`
- Aggregation: `COUNT`
- GroupBy: `$.agent_name`
- Emission: `handlers/agents.go:170` RegisterAgent handler, after successful version creation

### 1d. `agent_deployments` -- Agents Deployed

Tracks the number of deployment operations (each deploy command counts as one).

- Event type: `agent_deploy`
- Aggregation: `COUNT`
- GroupBy: `$.agent_name`
- Emission: `handlers/deploy.go` DeployAgent handler, after successful namespace creation

---

## 2. Customer Management

### Account-to-Customer Mapping

| Astro | OpenMeter |
|---|---|
| `accounts.id` (UUID) | Customer `key` + subject key |
| `accounts.name` | Customer `name` |
| `accounts.type` | Customer `metadata.type` |
| Owner email (from WorkOS) | Customer `primaryEmail` |

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

All events use CloudEvents format with `subject` = `account.id`:

```json
{
  "id": "<uuid>",
  "source": "astro-server",
  "specversion": "1.0",
  "type": "<event_type>",
  "subject": "<account.id>",
  "time": "<RFC3339>",
  "data": { ... }
}
```

Event ingestion is **async fire-and-forget** (buffered channel or goroutine) so it never blocks request handling. Failed events are logged and can be retried.

---

## 4. Plans and Subscriptions

### Example Plans

**Free**
- `messages`: metered entitlement, 10K/month, hard limit
- `compute`: metered entitlement, 100 CU-hours/month, hard limit
- `agent_registrations`: metered entitlement, 5/month, hard limit
- `agent_deployments`: metered entitlement, 10/month, hard limit
- Flat fee: $0

**Pro**
- `messages`: tiered pricing -- first 50K at $0, then $0.002/message
- `compute`: usage-based, $0.05/CU-hour
- `agent_registrations`: unlimited (soft limit)
- `agent_deployments`: metered entitlement, 100/month
- Flat fee: $49/month

**Enterprise**
- Custom rate cards, negotiated per customer
- Soft limits on all meters

### Subscription Creation

When an account upgrades (triggered from billing UI or admin action):

```
POST /api/v1/subscriptions
{
  "name": "<account.name> - <plan_name>",
  "customerId": "<openmeter_customer_id>",
  "plan": { "key": "<plan_key>", "version": 1 },
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
- ~~Message sync~~: Not needed -- messaging container emits directly to OpenMeter.

---

## 7. Implementation Order

| Phase | Work | Files |
|---|---|---|
| **Phase 1** | OpenMeter client + customer creation on account create + backfill migration | `internal/openmeter/client.go`, `handlers/accounts.go`, `schema.sql`, migration script |
| **Phase 2** | Inline meters: `agent_deployments` + `agent_registrations` event emission | `internal/openmeter/events.go`, `handlers/deploy.go`, `handlers/agents.go` |
| **Phase 3** | Background meters: `compute` heartbeat + `messages` Galileo sync | `internal/openmeter/sync.go` |
| **Phase 4** | Plan/subscription management + entitlement enforcement middleware | `internal/openmeter/middleware.go`, plan definitions in OpenMeter |

---

## Open Questions

- [x] Should customer creation failure block account creation, or be eventually consistent? **Eventually consistent.** Log error, continue, reconcile via background job.
- [x] Do we need per-user metering within an organization, or only per-account? **Per-account only.** Single subject key per account, no per-user breakdown.
- [x] Where does plan selection UI live -- astro-client, astro-queen, or both? **Auto-assign default plan on account creation.** Plan selection UI in both astro-client and astro-queen later.
- [x] Should entitlement checks return degraded responses (e.g. read-only mode) or hard 403s? **Degraded responses.** Allow reads, block writes/deploys. Return 402 with entitlement details so the client can show upgrade prompts.
- [x] What is the API key auth mechanism for OpenMeter -- env var, secret, or service account? **No auth.** OpenMeter is internal/private, accessed directly via `OPENMETER_URL`.
- [x] Should the Galileo token sync be real-time (webhook from Galileo) or polled? **N/A.** Galileo is not used for metering at all.
- [x] Do we need OpenMeter's notification system for threshold alerts (75%, 100% usage)? **Not yet.** No notifications for now.
- [ ] What env vars does the messaging container need? At minimum: `OPENMETER_URL`, `OPENMETER_SUBJECT` (account ID). No API key needed (no auth). Injected by the deploy handler into the messaging container spec.
