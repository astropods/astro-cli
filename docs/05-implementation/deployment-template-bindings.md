# Deployment Template — Knowledge Store Bindings

> **Stale in several places — verify against code before relying on this
> doc.** Confirmed wrong as of this pass: there is no `knlg0-*` K8s DNS
> namespace anywhere in the codebase. A bound entry's host resolves via
> `boundKnowledgeHost` (`internal/deployer/deployer.go`) to either the
> store's own PrivateLink endpoint DNS or its decrypted `HOST` credential —
> see [`knowledge-store.md`](knowledge-store.md) for the correct mechanism.
> The generic top-level `editable` field this doc describes doesn't exist on
> the real `TemplateResponse` either — see
> [`deployment-template-post.md`](deployment-template-post.md)'s banner.
> The binding request/response shape (`bindings.knowledge`, `ResolvedBindings`)
> and the `knowledge_store_bindings` table are still accurate, field-for-field
> — the one naming difference is the response struct is `KnowledgeBindingInfo`
> in code, not `KnowledgeBinding` as below.

Scope: adding knowledge store binding support to the interactive POST template endpoint. Depends on the POST endpoint (deployment-template-post.md) being in place.

---

## What Changes

Knowledge entries in `astropods.yml` are topology requirements — they declare what the agent needs, not how it's provisioned. Today the template endpoint always provisions knowledge as containers. Bindings let the caller swap any knowledge entry for a managed store instead.

When a binding is present, the knowledge entry stays in the template with its `binding` field set to the store ARN and container fields zeroed — identical to how the deployment spec is stored. Credential variables and editable fields for the bound entry are removed. The agent container receives environment variables pointing at the managed store's DNS and credentials. When no binding is present, the entry is self-hosted as a container exactly as today.

---

## Template Endpoint

### Request addition

```json
{
  "bindings": {
    "knowledge": {
      "db": "arn:knowledge:acme:postgres-main"
    }
  }
}
```

| Field | Purpose |
|-------|---------|
| `bindings.knowledge` | Knowledge entry name → managed store ARN. Each key must match a key in the agent's `knowledge` map. |

### How bindings shape the template

When `bindings.knowledge["db"] = "arn:knowledge:acme:postgres-main"`:

1. **Server resolves the ARN** — looks up the store in `knowledge_stores`, validates it belongs to the account, checks provider match against the spec's `knowledge.db.provider`, checks `status = ready`.

2. **`knowledge.db` in the template** gets its `binding` field set to the store ARN. All other fields are zeroed — `image`, `replicas`, `resources`, `persistent`, `volume`, `storage`, `environment`, `healthcheck`, `update`, `endpoints`. The provider registry has the port info; the reference resolver reads it from there at deploy time.

3. **Credential variables for the bound provider are removed** from both root `variables` and `template.variables`. The managed store owns its credentials.

4. **Editable fields** for the bound entry (`knowledge.db.replicas`, `knowledge.db.resources`, `knowledge.db.storage`, etc.) are removed from the `editable` list.

5. **Agent env var references still resolve** — `POSTGRES_HOST = ${knowledge.db.host}` stays in the agent's environment block. At deploy time, the reference resolves via `boundKnowledgeHost` to the store's PrivateLink endpoint DNS if it has one, otherwise its own decrypted `HOST` credential — never a local container service.

6. **Binding metadata is returned** in a new top-level `bindings` field in the response so the client knows what's bound:

```json
{
  "bindings": {
    "knowledge": {
      "db": {
        "arn": "arn:knowledge:acme:postgres-main",
        "name": "postgres-main",
        "provider": "postgres",
        "status": "ready"
      }
    }
  }
}
```

When a binding is absent, the knowledge entry is produced exactly as today — full container config, credentials in variables, all editable fields present.

### Validation

| Category | Rule |
|----------|------|
| ARN existence | ARN must resolve to a store in the caller's account |
| Provider match | Store's `provider` must match the knowledge entry's `provider` in the spec |
| Store status | Store must be `ready` (not `provisioning` or `error`) |
| Entry existence | Key must exist in the agent's `knowledge` map |

Binding errors appear in `validation.errors` alongside variable and adapter errors:

```json
{ "field": "bindings.knowledge.db", "message": "store is not ready (status: provisioning)" }
{ "field": "bindings.knowledge.cache", "message": "provider mismatch: entry declares qdrant, store is redis" }
{ "field": "bindings.knowledge.logs", "message": "no knowledge entry 'logs' in agent spec" }
```

### Prefill

When `deployment_id` is provided, stored bindings (from `knowledge_store_bindings` table) are included as if the client had sent them in `bindings`. Request-level `bindings` override stored bindings when both are present for the same key.

---

## Spec Struct Changes

### New: `KnowledgeBinding` (template response only)

```go
type KnowledgeBinding struct {
    ARN      string `json:"arn"`
    Name     string `json:"name"`
    Provider string `json:"provider"`
    Status   string `json:"status"`
}
```

### New: `TemplateBindings` (request)

```go
type TemplateBindings struct {
    Knowledge map[string]string `json:"knowledge,omitempty"` // entry name → store ARN
}
```

### New: `ResolvedBindings` (response)

```go
type ResolvedBindings struct {
    Knowledge map[string]KnowledgeBinding `json:"knowledge,omitempty"` // entry name → resolved binding
}
```

### Modified: `TemplateRequest`

```go
type TemplateRequest struct {
    // ... existing fields ...
    Bindings *TemplateBindings `json:"bindings,omitempty"`
}
```

### Modified: `TemplateResponse`

```go
type TemplateResponse struct {
    // ... existing fields ...
    Bindings *ResolvedBindings `json:"bindings,omitempty"`
}
```

### Modified: `DeploymentKnowledge`

```go
type DeploymentKnowledge struct {
    // ... all existing fields ...
    Binding string `json:"binding,omitempty" yaml:"binding,omitempty"` // store ARN
}
```

`Binding` is empty for self-hosted entries. When set, all other fields are zero-valued — the managed store provides everything. The reference resolver reads port info from the provider registry at deploy time.

The template and stored deployment spec are identical — bound entries stay in `knowledge` with `binding` set and container fields zeroed.

---

## Schemas

### Template request

An agent with two knowledge entries (`db` and `cache`), binding `db` to a managed store:

```json
{
  "build": "abc123",
  "adapters": ["slack"],
  "variables": {
    "SLACK_BOT_TOKEN": { "value": "xoxb-..." }
  },
  "bindings": {
    "knowledge": {
      "db": "arn:knowledge:acme:postgres-main"
    }
  }
}
```

### Template response / stored deployment spec

The `template` field in the response is the `deployment/v1` spec — identical to what gets stored after deploy. `db` has `binding` set and container fields zeroed; `cache` is a full container definition:

```json
{
  "spec": "deployment-template/v1",
  "template": {
    "spec": "deployment/v1",
    "agent": { "..." },
    "knowledge": {
      "db": {
        "binding": "arn:knowledge:acme:postgres-main"
      },
      "cache": {
        "provider": "qdrant",
        "image": "qdrant/qdrant:v1.8.0",
        "endpoints": { "http": { "port": 6333 }, "grpc": { "port": 6334 } },
        "replicas": 1,
        "resources": { "cpu": "500m", "memory": "512Mi" },
        "persistent": true,
        "volume": "/qdrant/storage",
        "storage": { "size": "10Gi" },
        "environment": { "..." },
        "healthcheck": { "..." },
        "update": { "..." }
      }
    },
    "variables": { "..." }
  },
  "bindings": {
    "knowledge": {
      "db": {
        "arn": "arn:knowledge:acme:postgres-main",
        "name": "postgres-main",
        "provider": "postgres",
        "status": "ready"
      }
    }
  },
  "variables": { "..." },
  "editable": ["..."],
  "validation": { "valid": true, "errors": [] }
}
```

---

## Reference Resolution

References like `${knowledge.db.host}` and `${knowledge.db.http.port}` appear in the agent's environment block. Resolution depends on whether the entry is bound or self-hosted.

### Self-hosted (unchanged)

The resolver reads from the `DeploymentKnowledge` struct directly:

| Reference | Resolves to | Source |
|-----------|-------------|--------|
| `${knowledge.db.host}` | `{agent}-knowledge-db.{agent-ns}.svc.cluster.local` | Service DNS in agent namespace |
| `${knowledge.db.http.port}` | `5432` | `endpoints.http.port` on the struct |

### Bound

The entry only has `binding: "arn:..."`. The resolver follows the ARN to the store record and provider registry:

| Reference | Resolves to | Source |
|-----------|-------------|--------|
| `${knowledge.db.host}` | The store's PrivateLink endpoint DNS, or its own `HOST` credential if it has no PrivateLink endpoint | `boundKnowledgeHost` (`internal/deployer/deployer.go`) — never a K8s service DNS name, since the platform creates no resources for a bound store |
| `${knowledge.db.http.port}` | `5432` | Store record → provider → provider registry default port |

Credentials resolve via `${}` references the same way host and port do:

| Reference | Resolves to | Source |
|-----------|-------------|--------|
| `${knowledge.db.credentials.user}` | `astro_admin` | Store record → `knowledge_store_credentials` |
| `${knowledge.db.credentials.password}` | `s3cret` | Store record → `knowledge_store_credentials` |
| `${knowledge.db.credentials.database}` | `agent_db` | Store record → `knowledge_store_credentials` |

The agent author controls the env var names in their spec — the reference just points at the store's credential value. No special copy or remapping logic needed.

### Validation vs resolution

**Template time** — structural validation only. The validator checks that referenced entries exist and endpoint names are valid for the provider. For bound entries, the provider is looked up from the store record. No DNS or credential resolution happens here.

**Deploy time** — actual resolution. The resolver follows the ARN to produce real DNS names, ports, and secret references. This requires DB access and is authoritative (a store that became unavailable since template time is caught here).

---

## Deploy Endpoint Changes

`POST /api/v1/deploy` continues to accept a fulfilled `deployment/v1` spec. Bound knowledge entries already have `binding` set and container fields zeroed in the spec (the template produces it this way).

When the deploy handler sees `knowledge.db.binding` is set:

1. **Validates** the binding (store exists, ready, provider match) — same checks as the template endpoint, but authoritative. A stale template with a now-failed store is caught here.

2. **Skips container creation** — no StatefulSet or Service created in the agent namespace for that entry.

3. **Resolves references differently** — `${knowledge.db.host}` resolves via `boundKnowledgeHost` (the store's PrivateLink endpoint DNS, or its own `HOST` credential) instead of `{agent}-knowledge-db.{agent-ns}.svc.cluster.local`. Port references (`${knowledge.db.http.port}`) resolve from the provider registry as usual.

4. **Resolves credentials** — `${knowledge.db.credentials.*}` references resolve from the store's `knowledge_store_credentials` record, same path as host and port resolution.

5. **Records the binding** — inserts into `knowledge_store_bindings` table.

6. **On undeploy** — deletes the binding row. The store continues running.

---

## Data Model

### New table: `knowledge_store_bindings`

```sql
CREATE TABLE public.knowledge_store_bindings (
    deployment_id      varchar(11)  NOT NULL,
    knowledge_name     varchar      NOT NULL,
    knowledge_store_id varchar(11)  NOT NULL,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (deployment_id, knowledge_name),
    FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    FOREIGN KEY (knowledge_store_id) REFERENCES public.knowledge_stores(id) ON DELETE RESTRICT
);
```

`ON DELETE CASCADE` on the deployment FK: when a deployment is deleted, its bindings are cleaned up automatically.

`ON DELETE RESTRICT` on the store FK: stores with active bindings cannot be deleted. The store DELETE endpoint returns `409 Conflict` with the list of bound deployments.

---

## Client Flow

### Binding picker

The deploy form renders a binding picker per knowledge entry in the agent's spec. Each picker shows:
- Available managed stores in the account (filtered by matching provider)
- Store status (ready / provisioning / error)
- Currently selected binding (if any, from prefill or user selection)

Selecting a store triggers a re-POST with the updated `bindings` map. The server returns the reshaped template — the bound knowledge entry has `binding` set and container fields zeroed, and its credential variables and editable fields are removed.

Clearing a binding re-POSTs without that entry in `bindings`, restoring the full container config and associated variables.

### Structural change

Binding selection is a structural change (like adapter toggle) — it triggers an immediate re-POST, not a debounced one, because it changes which knowledge entries, variables, and editable fields are present in the template.

---

## What Is Not Built Here

- Multi-store binding (one knowledge entry → multiple stores)
- Binding to external stores (bring-your-own credentials)
- Model or tool bindings (only knowledge entries support binding)
- Automatic binding suggestion
- CLI `--bind` flag
- Cross-account store sharing
