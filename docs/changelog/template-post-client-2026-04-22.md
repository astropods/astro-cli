# Interactive POST Template — Client Migration

## Summary

Migrates the deploy form from the static GET template endpoint to the interactive POST endpoint. The server now owns all template shaping — adapter selection, variable filling, schedule validation, and auth config — and returns a ready-to-deploy spec. The client sends inputs, gets back a shaped template with inline validation, and posts it to `/deploy` as-is.

This eliminates client-side fulfillment logic (`fulfillTemplate`, `buildInterfacesPayload`, hardcoded adapter definitions) and makes the deploy form fully server-driven.

## Design

### Request/Response Symmetry

`TemplateRequest` and `TemplateResponse` share a symmetric `interfaces` structure:

```
Request:  { interfaces: { adapters, auth }, variables, schedules }
Response: { template, variables, interfaces, schedules, validation }
```

- **`interfaces`** — adapters + auth at the response root (user-editable subset). The full `DeploymentInterfaces` (image, resources, endpoints) lives inside `template` for the deploy POST.
- **`schedules`** — ingestion name → cron expression. Server applies them before cron validation, eliminating client-side post-hoc patching.
- **`variables`** — full schema with descriptions, labels, placeholders, and the new `fields` sub-field definitions for object-typed variables.

### Server-Driven Object Variable Fields

Variables with `datatype: "object"` can declare a `fields` map describing their JSON sub-fields. Each field carries label, description, placeholder, datatype (`"csv"` for comma-separated arrays), and optional flag. The server emits `SLACK_CONFIG` with this schema; the client reads it to render individual inputs instead of hardcoded virtual fields.

```go
type VariableField struct {
    Label       string
    Description string
    Placeholder string
    Datatype    string  // "csv", "string", "boolean"
    Optional    bool
}
```

Client form keys use `PARENT.field_name` convention (e.g. `SLACK_CONFIG.actionable_reactions`). Serialization assembles sub-field values back into JSON. The deployment spec and runtime are unchanged — `SLACK_CONFIG` remains a JSON string env var.

### Template Shaping Pipeline

`ShapeTemplate` applies all user inputs to a deep copy of the base template before validation:

1. **Interfaces** — sets adapters, flips Slack token optionality, enables HTTP expose for web, applies auth
2. **Variable stripping** — removes variables targeting exclusively non-selected adapters
3. **Variable filling** — applies user-provided values and refs
4. **Schedule shaping** — applies cron expressions to ingestion triggers
5. **Validation** — required variables, cron expressions
6. **Response construction** — promotes interfaces, schedules, and variable schema to root; strips template-only fields from the deployment/v1 body

### Server-Side Caching

`templateCache` (`sync.Map` + TTL) caches generated base templates per agent+build+deployment. Template generation (DB lookups, spec parsing) runs once; subsequent reshapes (adapter toggle, variable fill) deep-copy from cache and apply `ShapeTemplate`.

### Secret Handling

Stored secret values without account variable refs are hidden in prefilled templates (they're encrypted blobs, useless to the UI). Secrets with refs show the ref so the vault picker can display which account variable was selected.

## Migration

No user action required. The GET template endpoints are still served for backward compatibility but are no longer called by the client. The deploy payload shape is unchanged — `deployment/v1` specs post to `/deploy` as before.
