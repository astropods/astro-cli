# Resource Manifest: Single Source of Truth for Spec → K8s Mapping

## Problem

Three independent code paths walk the same `AstroDeploymentSpec` to decide what K8s resources should exist:

| Path | File | Purpose |
|------|------|---------|
| Applier | `internal/k8s/spec_applier.go` | Creates K8s resources |
| Normalizer | `internal/deploymentstore/normalized.go` | Writes workload/service/ingress rows to DB |
| Orphan cleanup | `internal/k8s/orphan_cleanup.go` | Builds expected name set, deletes anything not in it |

Each independently re-derives: "for component X, create resource of kind Y with name Z". They've diverged multiple times:

- Collector changed from sidecar to standalone deployment — normalizer still inserted it as a sidecar
- Orphan cleanup listed collector as expected Service but not expected Deployment — deleted it immediately after creation
- Messaging moved from regular container to init container — tests in 4 files needed updating

The conformance test (`TestApplyDeploymentSpec_WorkloadNamesMatchNormalized`) catches some of this, but the root cause is structural: there is no shared authority for "what resources does this spec produce?"

## Design: `PlanResources`

A pure function in the `deployment` package that walks the spec once and returns a declarative manifest:

```go
// deployment/manifest.go

func PlanResources(ds *spec.AstroDeploymentSpec, cfg PlanConfig) *ResourceManifest
```

### Types

```go
type ResourceKind string

const (
    KindDeployment  ResourceKind = "Deployment"
    KindStatefulSet ResourceKind = "StatefulSet"
    KindService     ResourceKind = "Service"
    KindIngress     ResourceKind = "Ingress"
    KindCronJob     ResourceKind = "CronJob"
    KindJob         ResourceKind = "Job"
    KindSidecar     ResourceKind = "Sidecar"  // not a K8s kind; lives inside agent pod
)

type ComponentRole string

const (
    RoleAgent     ComponentRole = "agent"
    RoleModel     ComponentRole = "model"
    RoleKnowledge ComponentRole = "knowledge"
    RoleTool      ComponentRole = "tool"
    RoleMessaging ComponentRole = "messaging"
    RoleCollector ComponentRole = "collector"
    RoleIngestion ComponentRole = "ingestion"
)

type PlannedResource struct {
    Name         string        // K8s resource name (from GenerateResourceName)
    Kind         ResourceKind
    Role         ComponentRole
    ComponentKey string        // map key within the spec (e.g. model name "llm"), empty for singletons
    Persistent   bool          // drives StatefulSet vs Deployment
    ParentName   string        // for Service/Ingress: the workload they attach to
}

type PlanConfig struct {
    IngressDomain          string
    IngestionIngressDomain string
    Namespace              string // needed for ingress host generation
}

type ResourceManifest struct {
    AgentName string
    Resources []PlannedResource
}
```

### What it captures vs. what it doesn't

**Captures** (structural decisions): resource names, kinds, which components produce workloads, sidecar vs standalone, StatefulSet vs Deployment, whether ingress exists.

**Does not capture** (consumer-specific details): images, env vars, resource limits, healthchecks, volumes, K8s labels, DB encryption. These stay in each consumer and are read from the original spec.

### Spec → Resource Mapping (consolidated from all three paths)

| Component | Condition | Resources |
|-----------|-----------|-----------|
| Agent | Always | Deployment + Service |
| Agent | `endpoints.*.expose.enabled` + domain | + Ingress |
| Model | `persistent=false` | Deployment + Service |
| Model | `persistent=true` | StatefulSet + Service |
| Knowledge | `persistent=false` | Deployment + Service |
| Knowledge | `persistent=true` | StatefulSet + Service |
| Tool | Always | Deployment + Service |
| Messaging | `interfaces.adapters` non-empty | Sidecar + Service |
| Messaging | web adapter + expose domain | + Ingress |
| Collector | `observability.enabled=true` | Deployment + Service |
| Ingestion | `trigger=schedule` + schedule set | CronJob |
| Ingestion | `trigger=startup` | Job |
| Ingestion | `trigger=webhook` | Deployment + Service |
| Ingestion | `trigger=webhook` + ingestion domain | + Ingress |
| Ingestion | `trigger=manual` | (none — annotation only) |

## Implementation

### Step 1: `PlanResources` + orphan cleanup migration

**Files:**
- `internal/deployment/manifest.go` — new, ~120 lines
- `internal/deployment/manifest_test.go` — new, table-driven tests
- `internal/k8s/orphan_cleanup.go` — replace `computeExpectedResourceNames` body

The orphan cleanup is the simplest consumer. Replace its body with:

```go
func computeExpectedResourceNames(ds *spec.AstroDeploymentSpec, ingressDomain, ingestionIngressDomain string) map[string]map[string]bool {
    manifest := deployment.PlanResources(ds, deployment.PlanConfig{
        IngressDomain: ingressDomain, IngestionIngressDomain: ingestionIngressDomain,
    })
    expected := map[string]map[string]bool{
        "Service": {}, "Deployment": {}, "StatefulSet": {},
        "Ingress": {}, "CronJob": {}, "Job": {},
    }
    for _, r := range manifest.Resources {
        if r.Kind == deployment.KindSidecar {
            continue
        }
        expected[string(r.Kind)][r.Name] = true
    }
    return expected
}
```

The existing conformance test validates this against the applier output.

**Tests:** `manifest_test.go` with fixtures for: agent-only, agent+model+knowledge, full spec (all components), observability disabled, persistent vs non-persistent knowledge, all ingestion trigger types.

### Step 2: Normalizer migration

**Files:**
- `internal/deploymentstore/normalized.go` — refactor `SaveNormalizedSpec` structural loop

Current pattern (repeated per component type):
```go
for name, model := range ds.Models {
    // decide persistent → statefulset or deployment
    // buildWorkload + insertWorkload
    // saveEndpoints
}
```

New pattern:
```go
manifest := deployment.PlanResources(ds, ...)
for _, r := range manifest.Resources {
    switch r.Role {
    case deployment.RoleModel:
        model := ds.Models[r.ComponentKey]
        w := buildWorkload(componentInput{kind: "model", key: r.ComponentKey, ...})
        // r.Kind already tells us StatefulSet vs Deployment
    case deployment.RoleCollector:
        // always a workload, never a sidecar — no room for divergence
    case deployment.RoleMessaging:
        // r.Kind == KindSidecar → insertSidecar
    }
}
```

The existing `buildWorkload`, `insertWorkload`, `insertSidecar`, `insertService`, `insertIngress` helpers stay unchanged. Only the loop structure changes.

### Step 3: Applier consistency assertion

**Files:**
- `internal/k8s/spec_applier.go` — add assertion at the end of `ApplyDeploymentSpec`

Before touching the applier's structure, add a runtime check:

```go
// At the end of ApplyDeploymentSpec, after all resources are created:
manifest := deployment.PlanResources(ds, deployment.PlanConfig{
    IngressDomain: a.ingressDomain, IngestionIngressDomain: a.ingestionIngressDomain,
    Namespace: a.namespace,
})
for _, r := range manifest.Resources {
    if r.Kind == deployment.KindSidecar { continue }
    found := false
    for _, res := range result.Resources {
        if res.Kind == string(r.Kind) && res.Name == r.Name { found = true; break }
    }
    if !found {
        log.Warn("manifest/applier divergence", "name", r.Name, "kind", r.Kind)
    }
}
```

This logs divergence in production without changing behavior — a safety net while we refactor.

### Step 4: Applier loop refactor (follow-up)

Replace the phase-based structure with a manifest-driven loop. This is the largest change and can happen across multiple PRs:

1. Group `manifest.Resources` by Role
2. For each group, iterate and build K8s objects using existing builder functions
3. Remove the per-phase sections that currently re-walk the spec

This is optional for correctness (the assertion catches divergence) but improves maintainability.

## Ordering and Dependencies

```
Step 1: manifest.go + orphan cleanup     ← fixes the immediate problem
Step 2: normalizer migration             ← can be same or next PR
Step 3: applier assertion                ← same PR as step 2, or separate
Step 4: applier refactor                 ← follow-up PRs, not urgent
```

Steps 1-3 can ship in a single PR. Step 4 is incremental and can happen over time.

## Removing the conformance test?

No. The conformance test (`TestApplyDeploymentSpec_WorkloadNamesMatchNormalized`) stays as a second line of defense. Even after migration, someone could introduce a bug in `PlanResources` itself, or a consumer could bypass it. The test catches both.
