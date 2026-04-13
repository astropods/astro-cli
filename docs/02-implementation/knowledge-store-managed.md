# Knowledge Store — Managed Implementation

Scope: account-level managed knowledge stores only. External (bring-your-own) and private connectivity are out of scope here.

---

## What Changes

Today, knowledge stores are lifecycle-tied to agent deployments. A `knowledge:` entry in `astropods.yml` provisions a StatefulSet/Deployment in the agent's namespace when the agent is deployed, and it disappears when the agent is undeployed.

After this change, a `KnowledgeStore` is a first-class account entity — provisioned independently, assigned an ARN, and persisting across agent deployments. Agents reference it by ARN; the platform wires the already-running store in at deploy time. Multiple agents can bind to the same store simultaneously.

Existing inline `knowledge:` entries (provider + container mode) continue to work unchanged. Binding a managed store to an agent is a deploy-time concern — no changes to `astropods.yml` or the `astro-spec` package.

---

## Phase 1 — Store Lifecycle

Core infrastructure: create, provision, list, delete managed stores independently of any agent.

### Data Model

#### New table: `knowledge_stores`

```sql
CREATE TABLE public.knowledge_stores (
    id          varchar(11)  NOT NULL PRIMARY KEY,   -- nanoid, same scheme as deployments
    account_id  uuid         NOT NULL REFERENCES public.accounts(id),
    name        varchar      NOT NULL,
    arn         varchar      NOT NULL UNIQUE,         -- arn:knowledge:{account_name}:{name}
    provider    varchar      NOT NULL,               -- postgres, qdrant, redis, neo4j
    status      varchar      NOT NULL DEFAULT 'provisioning',
    namespace   varchar      NOT NULL,               -- k8s namespace for the store
    storage            varchar      NOT NULL DEFAULT '10Gi',
    public             boolean      NOT NULL DEFAULT false,
    public_host        varchar,                             -- assigned once LB hostname is available
    error              text,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now(),
    UNIQUE (account_id, name)
);
```

Status lifecycle: `provisioning` → `ready` | `error`

### K8s Resources

Managed stores live in a dedicated namespace per account: `knlg0-{account-id}` (e.g., `knlg0-550e8400-e29b-41d4-a716-446655440000`). This isolates store infrastructure from agent workloads and allows independent lifecycle management.

For each managed store, the platform creates:

1. **StatefulSet** — one replica, uses the same builder as today (`BuildStatefulSet` in `k8s/statefulset.go`). Labels include `astro.io/store-id` and `astro.io/arn`.
2. **PersistentVolumeClaim** — via the StatefulSet's `volumeClaimTemplates`, sized to `knowledge_stores.storage`. Storage class from account config or cluster default.
3. **Service (private)** — ClusterIP service. Name: `{store-id}`. DNS: `{store-id}.knlg0-{account-id}.svc.cluster.local`.
4. **Service (public, if `public=true`)** — additional LoadBalancer service. `public_host` is assigned immediately at creation time (`{name}.{account-slug}.astropods.ai`) — the platform controls this domain so no LB hostname is needed to reserve it. The watcher creates the DNS CNAME once the LB hostname is available, then marks the store `ready`.

The knowledge namespace is created on-demand when the first store is provisioned for an account. Namespace labels: `astro.io/account-id`, `astro.io/namespace-type=knowledge`.

Provider images and mount paths come from the existing `ProviderRegistry` in `packages/astro-spec/provider.go`. No new provider entries needed — the same images are reused.

### Credentials

#### Storage

The DB is the authoritative store for credentials. The K8s Secret is a derived copy — it can always be recreated from the DB.

New table:

```sql
CREATE TABLE public.knowledge_store_credentials (
    knowledge_store_id  varchar(11)  NOT NULL REFERENCES public.knowledge_stores(id),
    key                 varchar      NOT NULL,   -- e.g. POSTGRES_PASSWORD
    value_encrypted     bytea        NOT NULL,   -- encrypted with platform KMS key
    PRIMARY KEY (knowledge_store_id, key)
);
```

At provisioning time: generate credentials → encrypt with platform KMS → insert rows → create K8s Secret from plaintext in the store's namespace.

Secret keys per provider:

| Provider | Keys |
|----------|------|
| `postgres` | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` |
| `qdrant` | `QDRANT__SERVICE__API_KEY` |
| `redis` | `REDIS_PASSWORD` |
| `neo4j` | `NEO4J_AUTH` (formatted as `neo4j/{password}`) |

#### K8s Secret lifecycle

The K8s Secret (`{store-id}-credentials`) is a derived artifact. If it is missing — due to accidental deletion, cluster migration, or a new cluster — the reconciler recreates it by decrypting the DB rows and applying the secret. This runs as part of the existing store reconciliation loop that already watches pod readiness.

On new cluster bootstrap, the server reconciles all `ready` stores: for each one, if the K8s Secret is absent, it is recreated before any agent can bind to the store.

#### Access

The StatefulSet references the secret via `envFrom` so the database process picks up credentials on startup. At Phase 2 deploy time, agent pods are given `secretKeyRef` references into the same secret — credentials never flow through the server at deploy time.

A retrieval endpoint for direct access (e.g. connecting to a public store from outside the cluster):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/accounts/{account}/knowledge/{name}/credentials` | Decrypt and return credentials |

Response decrypts from the DB at request time — does not depend on the K8s Secret existing.

### API

All under the existing auth middleware.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/accounts/{account}/knowledge` | Create managed store |
| `GET` | `/v1/accounts/{account}/knowledge` | List stores |
| `GET` | `/v1/accounts/{account}/knowledge/{name}` | Get store (status, ARN, host if public) |
| `DELETE` | `/v1/accounts/{account}/knowledge/{name}` | Delete store |
| `GET` | `/v1/accounts/{account}/knowledge/{name}/events` | Deployment events (SSE) |
| `GET` | `/v1/accounts/{account}/knowledge/{name}/logs` | Container logs (SSE) |
| `GET` | `/v1/accounts/{account}/knowledge/{name}/credentials` | Retrieve credentials |

**POST body:**
```json
{ "name": "postgres-main", "provider": "postgres", "storage": "20Gi", "public": false }
```

**GET (single) response:**
```json
{
  "arn": "arn:knowledge:acme:postgres-main",
  "provider": "postgres",
  "status": "ready",
  "storage": "20Gi",
  "public": true,
  "public_host": "postgres-main.acme.astropods.ai",
  "created_at": "..."
}
```

**Events** (`/events`) streams StatefulSet pod events as SSE — same format used by the existing deployment events endpoint. Used by the CLI to tail provisioning progress.

**Logs** (`/logs`) streams stdout/stderr from the store pod — same log streaming implementation used for agent pods.

**DELETE** is rejected with `409 Conflict` if the store has active bindings (Phase 2).

### CLI

New top-level command group `ast knowledge` added to `apps/astro-cli/cmd/`.

```
ast knowledge create --name <name> --provider <provider> [--storage <size>] [--public]
ast knowledge list
ast knowledge status <name-or-arn>
ast knowledge logs <name-or-arn>
ast knowledge credentials <name-or-arn>
ast knowledge delete <name-or-arn>
```

`ast knowledge create` prints the assigned ARN on success, then streams events from `/events` until status reaches `ready` or `error` — same pattern as `ast deploy`. With `--public`, the assigned domain is printed once `public_host` is populated.

`ast knowledge logs` streams from `/logs` — same behaviour as `ast logs` for agents.

### Provisioning Flow

```
ast knowledge create --name postgres-main --provider postgres --storage 20Gi [--public]
  │
  ▼
POST /v1/accounts/acme/knowledge
  │
  ├─ Validate: name unique for account, provider in registry
  ├─ Generate store ID (nanoid) and ARN
  ├─ Insert knowledge_stores row (status=provisioning)
  ├─ Ensure account knowledge namespace exists
  ├─ Create K8s StatefulSet + ClusterIP Service in knlg0-{account-id} namespace
  ├─ If public: assign public_host immediately, create LoadBalancer Service
  └─ Return 202 with ARN (and public_host if public); CLI streams /events

Background watcher (existing deployment watcher pattern):
  ├─ Watch StatefulSet pod readiness
  ├─ If public: wait for LB hostname → create DNS CNAME {public_host} → NLB hostname
  ├─ On ready: UPDATE knowledge_stores SET status='ready'
  └─ On failed: UPDATE knowledge_stores SET status='error', error=<message>
```

The background watcher reuses the existing pod readiness watch infrastructure used for agent deployments. It watches all namespaces with label `astro.io/namespace-type=knowledge`.

---

## Phase 2 — Agent Binding

Bind managed stores to agent deployments at deploy time and inject connection env vars.

### Data Model Addition

#### New table: `knowledge_store_bindings`

Tracks which deployments reference which stores. Used to prevent deletion of in-use stores and to surface store health in deployment views.

```sql
CREATE TABLE public.knowledge_store_bindings (
    id                 serial       PRIMARY KEY,
    knowledge_store_id varchar(11)  NOT NULL REFERENCES public.knowledge_stores(id),
    deployment_id      varchar(11)  NOT NULL REFERENCES public.deployments(id),
    entry_name         varchar      NOT NULL,  -- the key in astropods.yml knowledge map
    UNIQUE (deployment_id, entry_name)
);
```

### Env Var Injection

Bindings are passed as a parameter at deploy time. The deploy handler in `apps/astro-server` resolves each bound entry before building the `DeploymentSpec`.

**Resolution path (server-side, in `apps/astro-server/internal/deployment/template.go`):**

1. Receive deploy request with `bindings: { "db": "arn:knowledge:acme:postgres-main" }`.
2. For each binding, look up `knowledge_stores` by ARN → get `id`, `provider`, `namespace`.
3. Construct service DNS: `{id}.knlg0-{account-id}.svc.cluster.local`.
4. Inject using the existing env var naming convention for that provider:

| Provider | Variables injected |
|----------|--------------------|
| `postgres` | `DB_HOST`, `DB_PORT=5432`, `DB_URL=postgres://...` |
| `qdrant` | `DB_QDRANT_HOST`, `DB_QDRANT_PORT=6333`, `DB_QDRANT_URL=http://...` |
| `redis` | `DB_REDIS_HOST`, `DB_REDIS_PORT=6379`, `DB_REDIS_URL=redis://...` |
| `neo4j` | `DB_NEO4J_HOST`, `DB_NEO4J_PORT=7474`, `DB_NEO4J_URL=bolt://...` |

The prefix is the binding entry name (uppercased), matching the existing multi-entry convention in `envresolver.go`. The entry must also exist in `astropods.yml` with a matching `provider` — the binding overrides where the service DNS points, but the spec still declares what type is expected.

### CLI Addition

`ast deploy` gains a `--bind` flag:

```
ast deploy --bind db=arn:knowledge:acme:postgres-main
```

Multiple `--bind` flags are allowed (one per knowledge entry). The CLI passes them as a `bindings` map in the deploy request body. The entry name (`db`) must match a knowledge entry key in `astropods.yml`.

### Deploy-Time Binding Flow

```
ast deploy --bind db=arn:knowledge:acme:postgres-main
  │
  ▼
POST /v1/accounts/acme/agents/my-agent/deploy
  body: { bindings: { "db": "arn:knowledge:acme:postgres-main" } }
  │
  ├─ Parse astropods.yml (unchanged)
  ├─ For each entry in bindings:
  │   ├─ Verify entry name exists in astropods.yml knowledge map
  │   ├─ Lookup knowledge_stores by ARN → must exist and status=ready
  │   ├─ Validate provider match (spec entry provider vs. store provider) → hard error if mismatch
  │   └─ Resolve service DNS for env var injection
  ├─ Build DeploymentSpec with resolved env vars (no new K8s resources for the store)
  ├─ Apply agent workloads to agent namespace (unchanged)
  ├─ Insert knowledge_store_bindings row
  └─ Return deployment ID
```

The agent's K8s namespace has no resources for the managed store — it only has the env vars pointing at the store's ClusterIP service in the knowledge namespace. Cross-namespace service access within the same cluster is standard K8s behavior.

On undeploy: delete the `knowledge_store_bindings` row. The store continues running.

### Provider Mismatch Validation

The agent's `astropods.yml` knowledge entry declares a `provider`. The bound store has its own `provider` in `knowledge_stores`. If they differ, the deploy is rejected:

```
--bind db=arn:knowledge:acme:postgres-main
astropods.yml knowledge.db.provider = qdrant

→ error: provider mismatch: entry "db" declares qdrant, store is postgres
```

If the spec entry has no `provider` set (e.g. it uses only a `container:` block), no mismatch check is performed — the binding is accepted as-is.

---

## What Is Not Built Here

- External stores (bring-your-own credentials)
- Private connectivity (PrivateLink, connector agent)
- Managed store sizing / vertical scaling after creation
- Multi-region or cross-cluster stores
- Backup / restore
- UI (Knowledge section) — API-first; UI follows separately
