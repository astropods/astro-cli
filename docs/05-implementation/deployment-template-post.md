# Deployment Template — Interactive POST Endpoint

> **Stale in several places — verify against code before relying on this
> doc.** Confirmed wrong as of this pass: the structs live in
> `apps/astro-server/internal/deployment/deployment_spec.go`, not
> `packages/astro-spec/deployment_spec.go`. `TemplateRequest` and
> `TemplateResponse` have grown well past what's shown below — real
> `TemplateRequest` adds `Revision`, `Interfaces`, `Models`, `Schedules`,
> `Bindings`, `Provisioning`, `Finalize`, `ClusterID`; real `TemplateResponse`
> replaces the generic `Editable []string` this doc describes with typed
> fields the client edits directly (`Interfaces`, `Models`, `Schedules`,
> `Provisioning`), and adds a `Signature` for the finalize/deploy handoff.
> `ShapeTemplate`'s real signature is
> `ShapeTemplate(ctx, base *AstroDeploymentSpec, req *TemplateRequest, opts *ShapeOptions) *TemplateResponse`.
> The narrative sections below (why POST replaced GET, the phased client
> migration) are still accurate context; the struct/field-level details are
> not. See [`deployment-template-bindings.md`](deployment-template-bindings.md)
> for the same caveat on the bindings extension.

Scope: converting the deployment template from a static GET to an interactive POST that accepts deploy-time inputs (adapters, variables), shapes the template accordingly, and returns inline validation. Knowledge store bindings are out of scope — they build on top of this endpoint in a follow-up.

---

## What Changes

Today, the deployment template is a GET endpoint. It takes no input — it reads the registered `astropods.yml` spec and returns a flat `deployment-template/v1` with placeholder references and empty slots. Deploy-time decisions like adapter selection are handled client-side: the template always emits an empty adapters list and optional Slack variables, the client fills in values locally, transforms the template into a `deployment/v1` spec (`fulfillTemplate()`), and POSTs to `/deploy`.

This means the client owns fulfillment logic (stripping template-only fields, merging variables, building the interfaces payload) and validation is deferred to the deploy endpoint. As deploy-time decisions grow more complex — knowledge store bindings will change template structure — this client-side approach doesn't scale.

After this change, the template endpoint becomes a POST. The client sends partial inputs — adapters, variables — and gets back a template shaped by those inputs plus validation results. The server does the fulfillment: `template` in the response already has values filled in, spec version set to `deployment/v1`, and template-only fields stripped. When `validation.valid: true`, the client takes `template` as-is and POSTs it to `/deploy`.

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
| `adapters` | Selected interface adapters (e.g. `["slack", "web"]`) |
| `variables` | Variable values or account-variable refs to pre-fill and validate |

### Response

```json
{
  "spec": "deployment-template/v1",
  "template": {
    "spec": "deployment/v1",
    "source": { "account": "acme", "name": "my-agent", "build": "abc123", "registry": "..." },
    "target": { "runtime": "kubernetes" },
    "agent": { "..." : "..." },
    "knowledge": { "..." : "..." },
    "interfaces": { "..." : "..." },
    "observability": { "..." : "..." },
    "variables": { "SLACK_BOT_TOKEN": { "..." : "..." }, "MY_VAR": { "..." : "..." } }
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
      { "field": "variables.SLACK_APP_TOKEN", "message": "required for slack adapter" }
    ]
  }
}
```

Top-level fields:

| Field | Purpose |
|-------|---------|
| `spec` | Response envelope version — always `deployment-template/v1` |
| `template` | A complete `deployment/v1` spec — directly postable to `/deploy` when valid |
| `variables` | Variable schema for the UI (description, datatype, display-as, options, secret, optional, targets) |
| `editable` | Which template fields the client may modify |
| `validation` | Current validity state + field-level errors |

### Two variable surfaces

Root-level `variables` carries the **schema** for rendering the form — description, optional, secret, targets, datatype, display-as, options. This is what the UI reads to build inputs.

`template.variables` carries the **fulfilled values** for deployment — only runtime fields (value, ref, targets, secret, optional). Template-only fields (description, datatype, display-as, options, default) are stripped. When the client sends `variables: { "X": { "value": "foo" } }`, the returned `template.variables.X.value` is `"foo"`.

The server does fulfillment on every POST. The client reads the schema, submits inputs, and when `valid: true` takes `template` as-is.

---

## How Inputs Shape the Template

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

1. The variable's `value` or `ref` field is set in the returned `template.variables.X`.
2. Validation checks the value against the variable's constraints (required, datatype).
3. Variables that reference account variables (`ref`) are validated for existence.

Variables not provided in the request retain their template defaults (if any) or remain empty.

### Prefill (deployment_id)

When `deployment_id` is provided:

1. The stored deployment's build is used (unless `build` is also provided).
2. Stored variable values are merged: non-secret values restored directly, secret values with refs restore the ref, secret values without refs left empty.
3. Stored adapters are merged into the template.
4. Stored ingestion schedules and display name are merged.

Request-level inputs (`adapters`, `variables`) override the prefilled values when both are present.

---

## Validation Model

Validation runs on every POST and returns all errors at once. The template is always returned regardless of validation state — the client gets both the shaped template and the problems to fix.

### Checks

| Category | Rule |
|----------|------|
| Required variables | Non-optional variables without `value` or `ref` produce an error |
| Adapter requirements | Slack selected → `SLACK_BOT_TOKEN` and `SLACK_APP_TOKEN` must have values |
| Variable refs | Account variable refs must exist in the account's variable store |
| Ingestion schedules | Cron expressions validated if present |

### What validation does NOT check

- Whether the agent image exists in the registry (checked at deploy time)
- Whether K8s resources can be created (checked at deploy time)
- Knowledge store bindings (future)

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
  ├─ POST {adapters, variables} ───────────►│ Full validation
  │◄─── template + valid: true ─────────────┤
  │                                         │
  ├─ POST /deploy (template as-is) ────────►│ Deploy
```

---

## Server Implementation

### Structs — `apps/astro-server/internal/deployment/deployment_spec.go`

As shipped (grown well past the original design below — see this file's
banner). The shape at time of writing:

```go
type TemplateRequest struct {
    Build        string                   `json:"build,omitempty"`
    DeploymentID string                   `json:"deployment_id,omitempty"`
    Revision     int                      `json:"revision,omitempty"`
    Interfaces   *TemplateInterfaces      `json:"interfaces,omitempty"`
    Variables    map[string]VariableInput `json:"variables,omitempty"`
    Models       map[string]string        `json:"models,omitempty"`
    Schedules    map[string]string        `json:"schedules,omitempty"`
    Bindings     *TemplateBindings        `json:"bindings,omitempty"`
    Provisioning *TemplateProvisioning    `json:"provisioning,omitempty"`
    Finalize     bool                     `json:"finalize,omitempty"` // response includes a deploy-time HMAC signature
    ClusterID    string                   `json:"cluster_id,omitempty"`
}

type TemplateResponse struct {
    Spec         string               `json:"spec"`
    Template     AstroDeploymentSpec  `json:"template"`
    Variables    map[string]Variable  `json:"variables,omitempty"`
    Models       []ModelSelection     `json:"models,omitempty"`
    Interfaces   TemplateInterfaces   `json:"interfaces"`
    Schedules    map[string]string    `json:"schedules"`
    Bindings     *ResolvedBindings    `json:"bindings,omitempty"`
    Provisioning TemplateProvisioning `json:"provisioning,omitzero"`
    Validation   TemplateValidation   `json:"validation"`
    Signature    string               `json:"signature,omitempty"`
}
```

The original design's generic `Editable []string` doesn't exist anymore —
editability is now expressed as typed fields the client edits directly
(`Interfaces`, `Models`, `Schedules`, `Provisioning`) rather than a list of
editable JSON paths into `template`.

### Template shaping — `apps/astro-server/internal/deployment/template.go`

Function `ShapeTemplate(ctx context.Context, base *AstroDeploymentSpec, req *TemplateRequest, opts *ShapeOptions) *TemplateResponse` (added `ctx` and `opts` since the original design; behavior below is still the shape of what it does):

1. Deep-copy the base template.
2. Apply adapter shaping — set `interfaces.adapters`, flip Slack variable optionality.
3. Fill variable values/refs from the request.
4. Split the result:
   - `Template` = base with values filled in, spec set to `deployment/v1`, template-only variable fields (description, datatype, display-as, options, default) stripped, editable removed.
   - Root `Variables` = full variable map with schema fields intact.
   - Root `Editable` = the editable fields list.
5. Run validation, populate `Validation`.

The existing `GenerateDeploymentTemplate()` is unchanged — it still produces the base template. `ShapeTemplate` is a post-processing step.

### POST handler — `apps/astro-server/handlers/deploy.go`

New `PostDeploymentTemplate(...)` handler:

1. Parse JSON body into `TemplateRequest` (empty body = `{}`).
2. If `req.DeploymentID` is set: look up deployment, verify membership, load stored vars/adapters/schedules. Merge into request (request-level inputs override stored values).
3. Call `generateTemplate()` with build from request (or deployment, or latest).
4. Call `deployment.ShapeTemplate(base, &req)`.
5. Return `TemplateResponse` as JSON (always JSON, no YAML option).

### Route registration — `apps/astro-server/main.go`

Register POST alongside existing GETs. The GETs remain unchanged for backward compatibility.

---

## Client Implementation

### API method — `apps/astro-client/src/lib/api.ts`

New method and types:

```typescript
interface TemplateRequest {
  build?: string;
  deployment_id?: string;
  adapters?: string[];
  variables?: Record<string, { value?: string; ref?: string }>;
}

interface TemplateResponse {
  spec: 'deployment-template/v1';
  template: DeploymentSpec;
  variables: Record<string, DeploymentVariable>;
  editable: string[];
  validation: { valid: boolean; errors: { field: string; message: string }[] };
}
```

### Migration strategy

Rather than rewriting the deploy form in one shot, migrate incrementally:

**Phase 1 — Server endpoint only.** Add the POST handler, register the route. GET endpoints unchanged. No client changes. The POST endpoint can be tested independently.

**Phase 2 — Client switches to POST.** Replace the GET fetch with POST. The `useDeployForm` hook re-POSTs when adapters or structural inputs change. Server validation errors surface alongside existing client-side validation. `fulfillTemplate()` is replaced — on submit, use `response.template` directly.

**Phase 3 — Remove client-side fulfillment.** Delete `fulfillTemplate()` and the client-side validation that the server now handles. The client becomes a thin form that reads schema from `response.variables` and submits inputs.

### Deploy form changes — `apps/astro-client/src/components/deploy/useDeployForm.ts`

The hook currently:
- Fetches template once via GET
- Manages all form state locally
- Calls `fulfillTemplate()` on submit to build the `deployment/v1` spec

After migration:
- Initial fetch = POST `{}` (or `{ deployment_id }` for redeploy)
- Adapter toggles trigger re-POST (structural change reshapes template)
- Variable changes are debounced — no re-POST on every keystroke, only on submit or after debounce
- `response.validation` drives the deploy button enabled state
- On submit: take `response.template` as-is, POST to `/deploy`

### Settings page — `apps/astro-client/src/pages/DeployedAgentSettings.tsx`

Currently uses `usePrefilledDeploymentTemplate` (GET with deploymentID path param). Switches to POST with `{ deployment_id }` in body.

---

## Backward Compatibility

The existing GET endpoints remain registered and return the legacy flat `AstroDeploymentSpec` format (with `variables` and `editable` inline). No behavioral change for existing GET consumers.

- `GET /agents/:account/:name/deployment-template` — unchanged.
- `GET /agents/:account/:name/deployment-template/:deploymentID` — unchanged.

The deploy endpoint (`POST /deploy`) is unchanged — it still accepts `deployment/v1` specs.

---

## What Is Not Built Here

- Knowledge store bindings (follow-up — builds on the request/response envelope defined here)
- Model or tool bindings
- Real-time validation via WebSocket (POST-based iteration is sufficient)
- CLI template workflow changes (CLI deploys non-interactively)
- Account variable ref validation (requires wiring in the account variable store — can be added incrementally)
