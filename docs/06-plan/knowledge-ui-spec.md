# Knowledge Stores — UI Specification

> **Shipped, with drift.** The knowledge UI this describes has been built
> (`apps/astro-client/src/pages/knowledge/**`), but some sections below
> describe features that were never built or have since been removed (a
> managed-store creation dialog, Logs/Events tabs) — don't treat those as
> current. For the as-built store model, see
> [`../05-implementation/knowledge-store.md`](../05-implementation/knowledge-store.md).
> Kept here as the original plan, not as current documentation.

Design reference for building the knowledge store management UI.

---

## Data Model

A knowledge store is a database (Postgres, Redis, Qdrant, Neo4j, Pinecone) that agents use for memory, vector search, caching, etc. There are two modes:

| Mode | Description |
|------|-------------|
| **managed** | Platform provisions and runs the DB in-cluster. Has storage, logs, events. |
| **external** | User brings their own DB. Platform stores encrypted credentials only. |

### Store Object

```json
{
  "id": "ks-abc123",
  "arn": "arn:knowledge:acct-xyz:my-store",
  "name": "my-store",
  "provider": "postgres",
  "mode": "managed | external",
  "status": "provisioning | connecting | pending-acceptance | ready | error",
  "storage": "10Gi",
  "public": true,
  "public_host": "my-store.team.astro.example.com",
  "endpoint": {
    "cloud_provider": "aws",
    "endpoint_service": "com.amazonaws.vpce.us-east-1.vpce-svc-xxx",
    "region": "us-east-1",
    "endpoint_id": "vpce-0abc123",
    "endpoint_dns": "vpce-0abc123.us-east-1.vpce.amazonaws.com",
    "status": "connecting | pending-acceptance | ready | error",
    "error": null
  },
  "error": "health check failed: connection refused",
  "created_at": "2026-04-15T12:00:00Z",
  "updated_at": "2026-04-15T12:01:00Z",
  "events": [
    { "type": "Normal", "reason": "Pulling", "message": "Pulling image postgres:16", "count": 1 }
  ]
}
```

`endpoint` is only present for external stores connected via PrivateLink. `events` are K8s-style events, only meaningful for managed stores.

---

## Statuses

| Status | Applies to | Meaning | Visual |
|--------|-----------|---------|--------|
| `provisioning` | managed | K8s StatefulSet is spinning up | Spinner / yellow dot |
| `connecting` | external (PrivateLink) | VPC endpoint is being created | Spinner / yellow dot |
| `pending-acceptance` | external (PrivateLink) | User must accept the endpoint in AWS console | Yellow dot + action callout |
| `ready` | both | Store is operational | Green dot |
| `error` | both | Something failed (see `error` field) | Red dot + error message |

### Status Transitions

```
Managed:    provisioning → ready
                         → error

External:   ready                          (direct connect, health check passed)
            error                          (direct connect, health check failed)
            connecting → pending-acceptance → ready    (PrivateLink flow)
                                           → error
```

---

## Providers

| Provider | Icon/Logo | Default Port | Credential Fields |
|----------|-----------|-------------|-------------------|
| postgres | PostgreSQL elephant | 5432 | Host, Port, Database, Username, Password |
| qdrant | Qdrant logo | 6333 | Host, Port, API Key |
| redis | Redis logo | 6379 | Host, Port, Password |
| neo4j | Neo4j logo | 7474 | Host, Port, Username, Password |
| pinecone | Pinecone logo | 443 | Host, API Key |
| mysql | MySQL dolphin | 3306 | Host, Port, Database, Username, Password |

---

## API Endpoints

Base: `GET/POST/DELETE /api/v1/accounts/:account/knowledge`

| Action | Method | Path | Response |
|--------|--------|------|----------|
| List stores | GET | `/knowledge` | `Store[]` |
| Get store | GET | `/knowledge/:name` | `Store` |
| Create managed | POST | `/knowledge` | `Store` (202) |
| Connect external | POST | `/knowledge/connect` | `Store` (200) |
| Delete store | DELETE | `/knowledge/:name` | `{ message }` |
| Get credentials | GET | `/knowledge/:name/credentials` | `{ KEY: "value", ... }` |
| Stream logs | GET | `/knowledge/:name/logs` | `text/plain` |
| Stream events | GET | `/knowledge/:name/events` | `text/event-stream` (SSE) |

---

## Pages & Views

### 1. Knowledge List Page

Shows all stores for the current account.

**Table columns:**
| Column | Source | Notes |
|--------|--------|-------|
| Name | `name` | Link to detail page |
| Provider | `provider` | Show icon + name |
| Mode | `mode` | Badge: "Managed" or "External" |
| Status | `status` | Colored dot + label (see status table) |
| Storage | `storage` | Only meaningful for managed ("10Gi"). Show "—" for external |
| Created | `created_at` | Relative time ("3 hours ago") |

**Empty state:** "No knowledge stores yet. Create one to give your agents a database."

**Actions:**
- "Create Store" button → opens create dialog
- "Connect External" button → opens connect dialog
- Per-row: kebab menu with "Delete" (confirmation required)

---

### 2. Create Managed Store Dialog

Fields:

| Field | Type | Required | Default | Validation |
|-------|------|----------|---------|------------|
| Name | text | yes | — | 1-63 chars, lowercase alphanumeric + hyphens, no leading/trailing/consecutive hyphens |
| Provider | select | yes | — | postgres, qdrant, redis, neo4j |
| Storage | text | no | 10Gi | Valid K8s quantity (e.g. 10Gi, 500Mi) |
| Public | toggle | no | off | Exposes the store with a DNS hostname |

On submit → `POST /knowledge`. Response status is `provisioning`. Redirect to detail page which shows live provisioning progress via SSE events.

---

### 3. Connect External Store Dialog

Two modes, toggled by a "PrivateLink" switch:

#### Standard connect (PrivateLink off)

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| Name | text | yes | Same as create |
| Provider | select | yes | postgres, qdrant, redis, neo4j, pinecone, mysql |
| Host | text | yes | Hostname or IP |
| Port | number | yes | 1-65535 |
| Database | text | conditional | Required for postgres, mysql |
| Username | text | conditional | Required for postgres, mysql, neo4j |
| Password | password | conditional | Required for postgres, mysql, redis, neo4j |
| API Key | password | conditional | Required for qdrant, pinecone |
| Skip health check | toggle | no | Default off |

Show/hide credential fields based on selected provider (see provider table above).

On submit → `POST /knowledge/connect`. If response has `status: "error"`, show warning banner: "Store created but health check failed: {error}. The store is not reachable — verify your credentials and network."

#### PrivateLink connect (PrivateLink on)

Same fields, except:
- **Host** label changes to "VPC Endpoint Service Name"
- **Host** placeholder: `com.amazonaws.vpce.us-east-1.vpce-svc-...`
- **Host** validation: must start with `com.amazonaws.vpce.`
- Health check toggle is hidden (automatically skipped)
- After submit, show a PrivateLink progress view (see below)

---

### 4. Store Detail Page

Tabs or sections:

#### Overview
- **Name**, **ARN** (copyable), **Provider**, **Mode**, **Status**, **Created**
- **Public Host** (if public, copyable)
- **Error** banner (if `status === "error"`, red, shows `error` field)
- **PrivateLink** section (if `endpoint` is present):
  - Service: `endpoint.endpoint_service`
  - Region: `endpoint.region`
  - Endpoint ID: `endpoint.endpoint_id`
  - DNS: `endpoint.endpoint_dns` (copyable — this is what agents connect to)
  - Status: `endpoint.status`

#### Credentials
- Fetched on-demand from `GET /knowledge/:name/credentials`
- Show as key-value list with masked values and a "reveal" toggle per field
- Copy button per value
- If 404: "Credentials not available (KMS was not configured when this store was created)"

#### Logs (managed only)
- Stream from `GET /knowledge/:name/logs`
- Auto-scroll, monospace, ANSI color support
- Hide tab for external stores

#### Events (managed only)
- Real-time via SSE from `GET /knowledge/:name/events`
- Show during provisioning as a timeline
- Event types: `Normal` (info icon), `Warning` (yellow warning icon)
- Format: `{reason}: {message}` with count badge if `count > 1`

---

### 5. PrivateLink Progress View

Shown after connecting with `--private-link`. This is a stepped progress indicator:

1. **Creating endpoint** — status: `connecting`, show spinner
2. **Waiting for acceptance** — status: `pending-acceptance`
   - Yellow callout: "Action required: accept the endpoint connection request in your AWS console"
   - Show endpoint service name and VPC endpoint ID (once available)
   - Link to AWS VPC console (if we can construct the URL from region)
3. **Ready** — status: `ready`
   - Green checkmark
   - Show endpoint DNS name (copyable)
4. **Error** — status: `error`
   - Red banner with `error` message

Poll `GET /knowledge/:name` every 3 seconds to update the status.

---

### 6. Delete Confirmation

Destructive action. Show confirmation dialog:

> **Delete knowledge store "{name}"?**
>
> This will permanently delete the store and all its data. This action cannot be undone.
>
> If this store has active agent bindings, deletion will be blocked.

Error case: 409 response means the store has active bindings — show: "This store is bound to active agents. Remove the bindings first."

---

## Validation Rules Reference

**Store name:**
- 1–63 characters
- Lowercase letters, digits, hyphens only
- Cannot start or end with a hyphen
- No consecutive hyphens

**Storage size:**
- Valid Kubernetes quantity: number + unit suffix
- Examples: `10Gi`, `500Mi`, `1Ti`, `20Gi`
- Must be greater than zero

**VPC Endpoint Service Name (when PrivateLink is on):**
- Must start with `com.amazonaws.vpce.`
- Format: `com.amazonaws.vpce.{region}.vpce-svc-{hex}`

---

## Conditional Field Logic

When the provider selection changes in the connect dialog, show only the relevant credential fields:

```
postgres → Host, Port, Database, Username, Password
mysql    → Host, Port, Database, Username, Password
redis    → Host, Port, Password
neo4j    → Host, Port, Username, Password
qdrant   → Host, Port, API Key
pinecone → Host, API Key (no Port field)
```
