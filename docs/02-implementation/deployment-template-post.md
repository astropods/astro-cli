# Deployment Template — Interactive POST Endpoint

Scope: evolving the deployment template endpoint from a static GET to an interactive POST that accepts deploy-time inputs, shapes the template accordingly, and returns inline validation.

---

## What Changes

Today, the deployment template is a GET endpoint. It takes no input — it reads the registered `astropods.yml` spec and returns a `deployment-template/v1` with placeholder references and empty slots. Deploy-time decisions like adapter selection are handled with workarounds: the template always emits an empty adapters list and optional Slack variables, the user fills in values client-side, and the deploy handler merges everything at submit time.

This breaks down with knowledge store bindings. A binding is a deploy-time decision that changes the template structurally: bound entries lose container config, credential variables shift to platform-managed, and editable fields change. The template shape is no longer derivable from the spec alone.

After this change, the template endpoint becomes a POST. The client sends partial inputs — bindings, adapters, variables — and gets back a template shaped by those inputs plus validation results. The client iterates until `valid: true`, then posts to `/deploy`. Adapters and bindings follow the same pattern: deploy-time overlays that shape what the template contains.

---

## Endpoint

**POST** `/api/v1/agents/:account/:name/deployment-template`

Replaces both existing GET endpoints (fresh template and prefilled template).

### Request

All fields optional. An empty body `{}` produces the same result as the current GET.

```json
{
  "build": "abc123",
  "deployment_id": "dep-xxx-xxx",
  "bindings": {
    "db": "arn:knowledge:acme:postgres-main"
  },
  "adapters": ["slack"],
  "variables": {
    "SLACK_BOT_TOKEN": { "value": "xoxb-..." },
    "MY_VAR": { "ref": "prod-api-key" }
  }
}
```

| Field | Purpose |
|-------|---------|
| `build` | Pin to a specific build ID instead of latest |
| `deployment_id` | Prefill from an existing deployment (replaces GET `/:deploymentID`) |
| `bindings` | Knowledge entry name → managed store ARN |
| `adapters` | Selected interface adapters (e.g. `["slack", "web"]`) |
| `variables` | Variable values or account-variable refs to pre-fill and validate |

### Response

```json
{
  "spec": "deployment-template/v1",
  "template": {
    "spec": "deployment-template/v1",
    "source": { "account": "acme", "name": "my-agent", "build": "abc123", "registry": "..." },
    "target": { "runtime": "kubernetes" },
    "agent": { "..." : "..." },
    "knowledge": { "..." : "..." },
    "interfaces": { "..." : "..." },
    "observability": { "..." : "..." },
    "variables": { "SLACK_BOT_TOKEN": { "..." : "..." }, "MY_VAR": { "..." : "..." } },
    "editable": ["agent.replicas", "agent.resources", "variables.*.value"]
  },
  "variables": {
    "SLACK_BOT_TOKEN": { "secret": true, "optional": false, "targets": ["interface.slack"], "description": "..." },
    "MY_VAR": { "value": "user-input", "targets": ["agent"], "description": "..." }
  },
  "editable": [
    "agent.replicas",
    "agent.resources",
    "variables.*.value"
  ],
  "validation": {
    "valid": false,
    "errors": [
      { "field": "variables.SLACK_APP_TOKEN", "message": "required for slack adapter" },
      { "field": "bindings.db", "message": "store is not ready" }
    ]
  }
}
```

Top-level fields:

| Field | Purpose |
|-------|---------|
| `spec` | Response format version |
| `template` | A complete `AstroDeploymentSpec` — 100% compatible with the deploy endpoint. Fill in variable values, POST to `/deploy`. |
| `variables` | Promoted copy of `template.variables` — the primary interaction surface for the UI |
| `editable` | Promoted copy of `template.editable` — which template fields the client may modify |
| `validation` | Current validity state + errors. The client iterates until `valid: true`. |

`template` is a self-contained `AstroDeploymentSpec` with user-provided values already filled in. When the client sends `variables: { "X": { "value": "foo" } }`, the returned `template.variables.X.value` is `"foo"`. When `valid: true`, the client takes `template` as-is and POSTs it to `/deploy` — no client-side fulfillment step needed.

Root-level `variables` carries the **schema** (description, optional, secret, targets, datatype, display-as, options) for rendering the form. `template.variables` carries the **fulfilled values** for deployment. The server does the fulfillment on every POST — the client only needs to read the schema and submit inputs.

---

## How Inputs Shape the Template

### Bindings

When `bindings.db = "arn:knowledge:acme:postgres-main"`:

1. **Server resolves the ARN** — looks up the store, validates it belongs to the account, checks provider match against the spec's `knowledge.db.provider`, checks `status = ready`.

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

Fields zeroed: `image`, `replicas`, `resources`, `persistent`, `volume`, `storage`, `healthcheck`, `update`, `environment`. The store already provides all of these.

`endpoints` stays populated from the provider registry — the reference resolver still needs port info for `${knowledge.db.http.port}`.

3. **Credential variables for the bound provider are removed** — the managed store owns its credentials; the agent gets them via cross-namespace secret injection at deploy time.

4. **Editable fields** for the bound entry (`knowledge.db.replicas`, `knowledge.db.storage`, etc.) are removed from the `editable` list.

5. **Agent env var references are unchanged** — `POSTGRES_HOST = ${knowledge.db.host}` stays in the template. The reference resolves to different DNS at deploy time (store's ClusterIP in the knowledge namespace vs. agent-namespace service).

When a binding is absent, the knowledge entry is produced exactly as today — full container config, credentials in variables, all editable fields present.

### Adapters

When `adapters = ["slack"]`:

1. `interfaces.adapters` is set to `["slack"]` in the template.
2. Slack variables (`SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`) become `optional: false`.
3. `SLACK_CONFIG` is included with its default value.

When `adapters` is empty or absent:

1. `interfaces.adapters` is `[]` (current behavior).
2. Slack variables are included as `optional: true` — no validation error if unfilled.

This replaces the current pattern where Slack variables are always emitted as optional and the deploy handler validates them only at submit time.

### Variables

When `variables.X = { "value": "..." }` or `variables.X = { "ref": "vault-name" }`:

1. The variable's `value` or `ref` field is set in the returned template.
2. Validation checks the value against the variable's constraints (required, datatype).
3. Variables that reference account variables (`ref`) are validated for existence.

Variables not provided in the request retain their template defaults (if any) or remain empty.

### Prefill (deployment_id)

When `deployment_id` is provided:

1. The stored deployment's build is used (unless `build` is also provided).
2. Stored variable values are merged into the template (same logic as current `GetPrefilledDeploymentTemplate`): non-secret values restored directly, secret values with refs restore the ref, secret values without refs left empty.
3. Stored adapters are merged into the template.
4. Stored bindings (from `knowledge_store_bindings` table) are included as if the client had sent them in `bindings`.
5. Stored ingestion schedules and display name are merged.

Request-level inputs (`bindings`, `adapters`, `variables`) override the prefilled values when both are present.

---

## Validation Model

Validation runs on every POST and returns all errors at once. The template is always returned regardless of validation state — the client gets both the shaped template and the problems to fix.

### Checks

| Category | Rule |
|----------|------|
| Required variables | Non-optional variables without `value` or `ref` produce an error |
| Adapter requirements | Slack selected → `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` must have values |
| Binding existence | ARN must resolve to a store in the caller's account |
| Binding provider match | Store's provider must match the knowledge entry's provider in the spec |
| Binding status | Store must be `ready` (not `provisioning` or `error`) |
| Variable refs | Account variable refs must exist in the account's variable store |
| Ingestion schedules | Cron expressions validated if present |

### What validation does NOT check

- Whether the agent image exists in the registry (checked at deploy time)
- Whether K8s resources can be created (checked at deploy time)
- Cross-account permissions for shared stores (future)

### Iterative flow

```
Client                                    Server
  │                                         │
  ├─ POST {} ──────────────────────────────►│ Return base template
  │◄─── template + errors: [missing vars] ─┤
  │                                         │
  ├─ POST {adapters: ["slack"]} ───────────►│ Shape template with adapters
  │◄─── template + errors: [SLACK_BOT_TOKEN required] ─┤
  │                                         │
  ├─ POST {adapters, variables, bindings} ─►│ Full validation
  │◄─── template + valid: true ─────────────┤
  │                                         │
  ├─ POST /deploy (fulfilled spec) ────────►│ Deploy
```

---

## Deploy Endpoint Changes

`POST /api/v1/deploy` continues to accept a fulfilled `deployment/v1` spec. The spec now may contain knowledge entries with a `binding` field.

When the deploy handler sees `knowledge.db.binding != nil`:

1. **Validates** the binding (store exists, ready, provider match) — same checks as the template endpoint, but authoritative.
2. **Skips container creation** — no StatefulSet or Service created in the agent namespace for that entry.
3. **Resolves DNS differently** — `${knowledge.db.host}` resolves to `{store-id}.knlg0-{account-id}.svc.cluster.local` instead of `{agent}-knowledge-db.{agent-ns}.svc.cluster.local`.
4. **Injects credentials** via cross-namespace secret reference — the agent pod gets `secretKeyRef` entries pointing at the store's credentials secret in the knowledge namespace.
5. **Records the binding** — inserts into `knowledge_store_bindings` table.
6. **On undeploy** — deletes the binding row. The store continues running.

### Data model

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

`ON DELETE RESTRICT` on the store FK enforces the rule that stores with active bindings cannot be deleted (409 Conflict from the store DELETE endpoint).

---

## Spec Struct Changes

### New: `TemplateResponse`

```go
type TemplateResponse struct {
    Spec       string                    `json:"spec"`                 // "deployment-template/v1"
    Template   AstroDeploymentSpec       `json:"template"`             // full deployment spec, compatible with /deploy
    Variables  map[string]Variable       `json:"variables,omitempty"`  // promoted from template for UI convenience
    Editable   []string                  `json:"editable,omitempty"`   // promoted from template for UI convenience
    Validation TemplateValidation        `json:"validation"`           // validity + errors
}
```

`Template` is a standard `AstroDeploymentSpec` — same struct used everywhere, directly postable to `/deploy`. Root-level `Variables` and `Editable` are the same references promoted so the UI doesn't need to dig into `template` for its primary interaction surface.

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

`Binding` is nil for internal (container-deployed) entries. When non-nil, container fields are zero-valued — the binding is the authority.

No changes to `AstroSpec` or `astropods.yml` parsing.

---

## Client Flow

### Fresh deploy

1. User opens deploy page for an agent.
2. Client POSTs `{}` → gets base template + variable schema + validation errors.
3. UI renders form from root `variables` (schema), adapter toggles, binding picker per knowledge entry.
4. User interacts — toggles adapter, selects binding, fills variable. Client re-POSTs with current inputs → server returns reshaped template (with values filled in) + updated validation.
5. When `validation.valid: true` → deploy button enabled. Client takes `template` as-is and POSTs to `/deploy`. No client-side fulfillment — the server already baked the values in.

### Redeploy / configure

1. Client POSTs `{ "deployment_id": "dep-xxx" }` → gets prefilled template with stored values already filled into `template`, plus variable schema at root.
2. Same iterative flow — user changes inputs, client re-POSTs, server returns updated template + validation.
3. Re-POST includes `deployment_id` (for base prefill) plus the user's overrides.

### Debouncing

Variable value changes are debounced client-side (no re-POST on every keystroke). Structural changes (adapter toggle, binding selection) trigger immediate re-POST since they reshape the template.

---

## Backward Compatibility

The existing GET endpoints remain registered during a transition period:

- `GET /agents/:account/:name/deployment-template` → internally delegates to the POST handler with empty inputs.
- `GET /agents/:account/:name/deployment-template/:deploymentID` → delegates with `{ "deployment_id": deploymentID }`.

Both return the legacy flat `AstroDeploymentSpec` format (with `variables` and `editable` inline) for response compatibility. The new structured response (`spec`, `template`, `variables`, `editable`, `validation` at root) is only returned by the POST endpoint.

The deploy endpoint (`POST /deploy`) accepts specs with or without `binding` fields. Specs without bindings deploy exactly as today.

---

## What Is Not Built Here

- Multi-store binding (one knowledge entry → multiple stores)
- Binding to external stores (bring-your-own credentials) — separate spec
- Model or tool bindings (only knowledge entries support binding)
- Real-time validation via WebSocket (POST-based iteration is sufficient)
- Automatic binding suggestion (e.g. "you have a postgres store, want to use it?")
- CLI template workflow changes (CLI deploys non-interactively; `--bind` flag passes bindings directly to the deploy endpoint)
