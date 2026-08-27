# Plan: Decouple Env Resolution from K8s Applier

> **Likely moot.** Neither the bug this plan patches around
> (`ManagedAnthropicAPIKey` being dropped by `ApplyDeploymentSpec`'s internal
> re-resolution) nor the patch itself exists in current code —
> `internal/k8s/applier.go`'s `ApplierConfig` has no such field, and the only
> remaining references anywhere are in test files. Whether the underlying
> env-resolution duplication this plan describes still exists elsewhere is
> unverified; re-check before reviving this plan rather than assuming it's
> current. Kept here as the original design thinking, not as a live proposal.

## Context

The managed Anthropic API key was never reaching pods because `ApplyDeploymentSpec` internally re-resolves the env from the spec, discarding any upstream credential injection. This was patched by threading `ManagedAnthropicAPIKey` through `ApplierConfig`, but the underlying coupling remains: env resolution is duplicated across multiple call sites, and adding a new managed credential requires touching every path.

## Problem

`ApplyDeploymentSpec` does two things: (1) resolve `${}`-references and build ConfigMap/Secret data, and (2) apply K8s resources. Every caller resolves env *before* calling the applier (for DB storage, drift hashing, etc.), then the applier throws that away and re-resolves internally.

### Current call sites that resolve env

| Caller | File | Also injects managed creds? |
|--------|------|-----------------------------|
| Deploy handler | `handlers/deploy.go:416` | Yes (handler-level) |
| Deployer worker | `internal/k8s/spec_applier.go:43` (inside `ApplyDeploymentSpec`) | Yes (via `ApplierConfig` patch) |
| Admin gRPC repair | `internal/admingrpc/server.go:1882` | No |
| Normalized spec repair | `internal/deploymentstore/normalized.go:778` | No |
| E2E / drift tests | Various | No |

The deploy handler and the applier both call `ResolveDeploymentSpecEnv`, producing two `ResolvedEnv` values for the same deploy. Only the handler's version gets managed creds (and is stored in DB). The applier's version is what K8s actually sees. They diverge silently when any post-resolution transform is added to one but not the other.

### Current flow (broken symmetry)

```
┌──────────────────────────────────────────────────────────────────┐
│                      Deploy Handler (HTTP)                       │
│                                                                  │
│  spec ──► ResolveDeploymentSpecEnv ──► injectManagedCredentials  │
│                                              │                   │
│                                    ResolvedEnv (with managed key)│
│                                              │                   │
│                                     SaveNormalizedSpec (DB)       │
│                                              │                   │
│                                     InsertDeployJob (queue)       │
└──────────────────────────────────────────────────────────────────┘
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Deployer Worker (async)                       │
│                                                                  │
│  Load spec from DB ──► RehydrateSecrets (from deployment_variables)
│          │                                                       │
│          ▼                                                       │
│  ApplyDeploymentSpec                                             │
│    └──► ResolveDeploymentSpecEnv  ◄── SECOND resolution          │
│    └──► (managed key only present via ApplierConfig patch)       │
│    └──► Build K8s Secret/ConfigMap/Deployments                   │
└──────────────────────────────────────────────────────────────────┘
```

The two resolution paths must stay manually synchronized. Every new managed credential, env transform, or post-processing step must be added to both, plus any other caller that resolves env independently.

## Design

Move env resolution out of the applier. The applier becomes a pure K8s applicator that receives pre-resolved data.

### Target flow (single resolution)

```
┌──────────────────────────────────────────────────────────────────┐
│                      Deploy Handler (HTTP)                       │
│                                                                  │
│  spec ──► ResolveDeploymentSpecEnv ──► injectManagedCredentials  │
│                                              │                   │
│                                    ResolvedEnv (with managed key)│
│                                              │                   │
│                                     SaveNormalizedSpec (DB)       │
│                                              │                   │
│                                     InsertDeployJob (queue)       │
└──────────────────────────────────────────────────────────────────┘
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Deployer Worker (async)                       │
│                                                                  │
│  Load spec from DB ──► RehydrateSecrets                          │
│          │                                                       │
│          ▼                                                       │
│  ResolveDeploymentSpecEnv ──► injectManagedCredentials           │
│          │                                                       │
│          ▼                                                       │
│  applier.Apply(ctx, ds, resolvedEnv)  ◄── no internal resolution │
│    └──► Build K8s Secret/ConfigMap/Deployments from resolvedEnv  │
└──────────────────────────────────────────────────────────────────┘
```

### Responsibility split

```
┌─────────────────────────┐     ┌──────────────────────────────────┐
│    Callers (Deployer,   │     │         Applier                  │
│    Handler, Admin gRPC) │     │                                  │
│                         │     │  Receives:                       │
│  - Parse spec           │     │    • AstroDeploymentSpec         │
│  - Rehydrate secrets    │────►│    • ResolvedEnv                 │
│  - Resolve env          │     │                                  │
│  - Inject managed creds │     │  Does:                           │
│  - Any post-processing  │     │    • Build K8s manifests         │
│                         │     │    • Apply to cluster             │
└─────────────────────────┘     └──────────────────────────────────┘
```

### API change

```go
// Before — applier owns resolution
func (a *Applier) ApplyDeploymentSpec(
    ctx context.Context,
    ds *spec.AstroDeploymentSpec,
) (*ApplyResult, error)

// After — caller owns resolution
func (a *Applier) ApplyDeploymentSpec(
    ctx context.Context,
    ds *spec.AstroDeploymentSpec,
    resolved *deployment.ResolvedEnv,
) (*ApplyResult, error)
```

Remove `ManagedAnthropicAPIKey` from `ApplierConfig` (the patch we just added). The applier no longer needs it because it no longer resolves env.

## Implementation

### Step 1 — Change `ApplyDeploymentSpec` signature

Add `resolved *deployment.ResolvedEnv` parameter. Remove the internal `ResolveDeploymentSpecEnv` call and the managed-key injection block. The method uses the passed-in `resolved` directly.

**File:** `internal/k8s/spec_applier.go`

Delete these lines from `ApplyDeploymentSpec`:
- `ResolveContext` construction
- `ResolveDeploymentSpecEnv` call
- Managed Anthropic key injection block

Keep everything after (secret name computation, K8s resource building).

### Step 2 — Remove `ManagedAnthropicAPIKey` from applier

**File:** `internal/k8s/applier.go`

Remove:
- `ManagedAnthropicAPIKey` from `ApplierConfig`
- `managedAnthropicAPIKey` from `Applier` struct
- Assignment in `NewApplier`

### Step 3 — Update `Deployer.Apply` to resolve + inject before calling applier

**File:** `internal/deployer/deployer.go`

```go
func (d *Deployer) Apply(ctx context.Context, dep *deploymentstore.Deployment) (*k8s.ApplyResult, error) {
    // ... load spec, rehydrate secrets (unchanged) ...

    rctx := deployment.ResolveContext{
        Namespace:  dep.Namespace,
        AgentName:  dep.AgentName,
        BuildID:    dep.BuildID,
        SecretName: deployment.GenerateSecretName(dep.AgentName, dep.BuildID),
    }
    resolved := deployment.ResolveDeploymentSpecEnv(&ds, rctx)
    injectManagedCredentials(resolved, d.Cfg)

    // ... build applier (remove ManagedAnthropicAPIKey from config) ...

    return applier.ApplyDeploymentSpec(ctx, &ds, resolved)
}
```

Move `injectManagedCredentials` from `handlers/deploy.go` to `internal/deployment/managed_credentials.go` so both the handler and deployer can import it without a circular dependency.

### Step 4 — Update deploy handler

**File:** `handlers/deploy.go`

The handler already resolves env and injects managed creds for DB storage. No change to that path. Just update the call in `ValidateDeployment` or any other handler path that calls `ApplyDeploymentSpec` directly (if any — currently none do, the handler only enqueues).

### Step 5 — Update admin gRPC and repair paths

**Files:**
- `internal/admingrpc/server.go` — already resolves env at line 1882; pass it through if it calls `ApplyDeploymentSpec` (currently it doesn't, it's for drift hashing only — no change needed)
- `internal/deploymentstore/normalized.go` — `RepairNormalizedSpec` resolves env for hash comparison only, doesn't call applier — no change needed

### Step 6 — Update tests

- `internal/k8s/spec_applier_test.go` — all `ApplyDeploymentSpec` calls need a `resolved` parameter. Most can pass `deployment.ResolveDeploymentSpecEnv(ds, rctx)` inline.
- `TestApplyDeploymentSpec_ManagedAnthropicKey` — move to deployer test or rewrite to test that the deployer passes managed creds through resolved env.
- E2E tests that call `ApplyDeploymentSpec` — update signatures.

### Step 7 — Clean up `ApplierConfig`

Remove `ManagedAnthropicAPIKey` from deployer's config wiring. The config field on `config.DeploymentConfig` stays (the deployer reads it directly).

## What this prevents

Adding a new managed credential (e.g. `ManagedOpenAIAPIKey`) requires changes in exactly two places:
1. `config.go` — add the config field
2. `internal/deployment/managed_credentials.go` — add the injection

No changes to `ApplierConfig`, `Applier`, `spec_applier.go`, or any K8s layer. The applier remains a pure applicator with no knowledge of credential sources.

## Migration

No runtime migration needed. This is a refactor internal to the server — no API changes, no DB changes, no spec changes.
