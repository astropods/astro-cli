# Deployment Template — Knowledge Store Bindings

Scope: adding knowledge store binding support to the interactive POST template endpoint. Depends on the POST endpoint (deployment-template-post.md) and managed knowledge stores (knowledge-store-managed.md Phase 1) being in place.

---

## What Changes

The POST template endpoint accepts adapters and variables as deploy-time inputs that shape the template. Knowledge store bindings are the next input type: a binding maps a knowledge entry from the agent's spec to a managed store by ARN. This is a structural change — a bound entry loses its container config, its credential variables are removed, its editable fields are pruned, and the deploy handler skips container creation entirely.

After this change, the client includes `bindings` in the template POST request. The server resolves each ARN, validates it, reshapes the knowledge entry in the template, adjusts variables and editable fields, and returns the result with binding-specific validation errors.

The deploy endpoint also changes: when it sees a `binding` on a knowledge entry, it skips container creation and wires the agent to the managed store's service DNS and credentials.

---

## Template Endpoint

### Request addition

```json
{
  "bindings": {
    "db": "arn:knowledge:acme:postgres-main"
  }
}
```

| Field | Purpose |
|-------|---------|
| `bindings` | Knowledge entry name → managed store ARN. Entry name must match a key in the agent's `knowledge` map. |

### How bindings shape the template

When `bindings.db = "arn:knowledge:acme:postgres-main"`:

1. **Server resolves the ARN** — looks up the store in `knowledge_stores`, validates it belongs to the account, checks provider match against the spec's `knowledge.db.provider`, checks `status = ready`.

2. **`knowledge.db` in the template** gets a `binding` object and loses container config:

```json
{
  "knowledge": {
    "db": {
      "provider": "postgres",
      "endpoints": { "http": { "port": 5432 } },
      "binding": {
        "arn": "arn:knowledge:acme:postgres-main",
        "provider": "postgres",
        "status": "ready"
      }
    }
  }
}
```

Fields zeroed: `image`, `replicas`, `resources`, `persistent`, `volume`, `storage`, `healthcheck`, `update`, `environment`. The managed store provides all of these.

`endpoints` stays populated from the provider registry — the reference resolver still needs port info for `${knowledge.db.http.port}`.

3. **Credential variables for the bound provider are removed** from both root `variables` and `template.variables`. The managed store owns its credentials; the agent gets them via cross-namespace secret injection at deploy time.

4. **Editable fields** for the bound entry (`knowledge.db.replicas`, `knowledge.db.resources`, `knowledge.db.storage`, etc.) are removed from the `editable` list.

5. **Agent env var references are unchanged** — `POSTGRES_HOST = ${knowledge.db.host}` stays in the template. The reference resolves to different DNS at deploy time (store's ClusterIP in the knowledge namespace vs. agent-namespace service).

When a binding is absent, the knowledge entry is produced exactly as today — full container config, credentials in variables, all editable fields present.

### Validation additions

| Category | Rule |
|----------|------|
| Binding existence | ARN must resolve to a store in the caller's account |
| Binding provider match | Store's `provider` must match the knowledge entry's `provider` in the spec |
| Binding status | Store must be `ready` (not `provisioning` or `error`) |
| Entry existence | Binding key must match a key in the agent's `knowledge` map |

Binding errors appear in `validation.errors` alongside variable and adapter errors:

```json
{ "field": "bindings.db", "message": "store is not ready (status: provisioning)" }
{ "field": "bindings.cache", "message": "provider mismatch: entry declares qdrant, store is redis" }
{ "field": "bindings.logs", "message": "no knowledge entry 'logs' in agent spec" }
```

### Prefill

When `deployment_id` is provided, stored bindings (from `knowledge_store_bindings` table) are included as if the client had sent them in `bindings`. Request-level `bindings` override stored bindings when both are present for the same entry name.

---

## Spec Struct Changes

### New: `KnowledgeBinding`

```go
type KnowledgeBinding struct {
    ARN      string `json:"arn" yaml:"arn"`
    Provider string `json:"provider" yaml:"provider"`
    Status   string `json:"status" yaml:"status"`
}
```

### Modified: `DeploymentKnowledge`

```go
type DeploymentKnowledge struct {
    // ... all existing fields ...
    Binding *KnowledgeBinding `json:"binding,omitempty" yaml:"binding,omitempty"`
}
```

`Binding` is nil for self-hosted (container-deployed) entries. When non-nil, container fields are zero-valued.

### Modified: `TemplateRequest`

```go
type TemplateRequest struct {
    // ... existing fields ...
    Bindings map[string]string `json:"bindings,omitempty"` // entry name → store ARN
}
```

### RFC-2 alignment

RFC-2 Section 6.2 (`knowledge`) gains the `binding` field (conditional) and `image` becomes conditional (required only when `binding` is absent). The existing statement about bound entries not appearing in the map is replaced: bound entries stay in the map with a `binding` object so `${}` references can still resolve.

---

## Deploy Endpoint Changes

`POST /api/v1/deploy` continues to accept a fulfilled `deployment/v1` spec. The spec now may contain knowledge entries with a `binding` field.

When the deploy handler sees `knowledge.db.binding != nil`:

1. **Validates** the binding (store exists, ready, provider match) — same checks as the template endpoint, but authoritative. A stale template with a now-failed store is caught here.

2. **Skips container creation** — no StatefulSet or Service created in the agent namespace for that entry.

3. **Resolves DNS differently** — `${knowledge.db.host}` resolves to `{store-id}.knlg0-{account-id}.svc.cluster.local` instead of `{agent}-knowledge-db.{agent-ns}.svc.cluster.local`.

4. **Injects credentials** via cross-namespace secret reference — the agent pod gets `secretKeyRef` entries pointing at the store's credentials secret in the knowledge namespace. Credential keys come from the provider registry (e.g. `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` for postgres).

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

Selecting a store triggers a re-POST with the updated `bindings` map. The server returns the reshaped template — the UI updates to show the bound state (no container config fields, no credential variables for that entry).

Clearing a binding re-POSTs without that entry in `bindings`, restoring the full container config.

### Structural change

Binding selection is a structural change (like adapter toggle) — it triggers an immediate re-POST, not a debounced one, because it changes which fields and variables are present in the template.

---

## What Is Not Built Here

- Multi-store binding (one knowledge entry → multiple stores)
- Binding to external stores (bring-your-own credentials) — separate spec
- Model or tool bindings (only knowledge entries support binding)
- Automatic binding suggestion (e.g. "you have a postgres store, want to use it?")
- CLI `--bind` flag (CLI deploys non-interactively; passes bindings directly to `/deploy`)
- Cross-account store sharing
