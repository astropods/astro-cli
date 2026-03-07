# Agent Interfaces Spec

**Date:** 2026-03-07
**Spec Version:** v1.1

## Summary

Agents on Astro previously had no way to declare whether they serve their own web frontend or whether they need the messaging sidecar. Every agent implicitly got a messaging sidecar — wasteful for agents that handle their own UI, and with no mechanism to expose the agent's own HTTP port via ingress.

`agent.interfaces` makes this explicit. The agent author declares capabilities (topology), the deployer configures how they're wired (deployment config).

## Design

### Topology vs. Deployment

The core insight: *what the agent is* vs. *how it runs* are separate concerns.

- **Topology** (`agent.interfaces`) — boolean flags declaring capability. Lives in `astropods.yml`.
- **Deployment config** (`dev.interfaces`, deployment spec) — adapters, ports, domains. Lives outside the agent topology.

### agent.interfaces

Two boolean fields on the agent block in `Container`:

```yaml
agent:
  build: ...
  interfaces:
    frontend: true   # agent serves its own web UI on port 80
    messaging: true  # agent speaks the messaging protocol
```

- Omitting `interfaces` entirely → backward compat (messaging enabled via `HasMessaging()`)
- `frontend: true` → platform creates an ingress to the agent on port 80
- `messaging: true` → platform deploys the messaging sidecar

Helper methods `Container.HasFrontend()` and `Container.HasMessaging()` encapsulate the nil-means-messaging-enabled default.

### dev.interfaces

Evolved from `string[]` to a structured `DevInterfaces` object:

```yaml
dev:
  interfaces:
    frontend:
      port: 3000          # agent serves locally on 3000, platform proxies 80 → 3000
    messaging:
      adapters: [slack]   # which adapters to run locally
```

A custom `UnmarshalYAML` on `DevInterfaces` supports the legacy flat array format — `interfaces: [slack, web]` is parsed as `messaging: { adapters: [slack, web] }`. Existing specs continue to work without changes.

### Server: template generation

`GenerateDeploymentTemplate` in `deployment/template.go` now reads `agent.interfaces`:

- When `HasMessaging()` is false, the `DeploymentInterfaces` block and Slack credential variables are omitted entirely.
- When `HasFrontend()` is true, the agent's HTTP endpoint is set to port 80 with `expose.enabled: true`.
- Backward compat: agents without `interfaces` still get the messaging block (nil means messaging enabled).

### Server: K8s translation

`spec_applier.go` creates an agent ingress when the agent has an exposed endpoint:

- Checks `ExposedEndpoint(ds.Agent.Endpoints)` — if present, builds an ingress to the agent service.
- Uses the same ingress domain, ACM cert, and ALB group as the messaging ingress.
- Emits a `ServiceEndpoint` with type `"frontend"` for the external URL.
- The messaging sidecar path (`ds.Interfaces != nil`) is unchanged — already guarded correctly.

### CLI consumers

All CLI commands that read `Dev.Interfaces` were updated to use the new struct path (`Dev.Interfaces.Messaging.Adapters`):

- `compose/builder.go` — messaging sidecar, playground, GRPC_SERVER_ADDR injection
- `cmd/dev.go` — web interface detection, local env setup
- `cmd/configure.go` — Slack token prompting
- `cmd/explain.go` — interface listing
- `cmd/repair.go` — scaffold config extraction
- Scaffold template (`astropods.yml`) — outputs structured `dev.interfaces`

## Migration

No migration required. Both `agent.interfaces` and `dev.interfaces` are fully backward compatible:

- Omitting `agent.interfaces` → messaging enabled (existing behavior)
- `dev.interfaces: [slack, web]` (legacy) → still parsed correctly via custom unmarshaler
- `dev.interfaces: { messaging: { adapters: [slack] } }` (new) → structured form for new specs
