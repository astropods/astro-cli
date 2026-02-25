# Scale-to-Zero Implementation Plan

Scales agent namespaces to zero replicas when idle to reduce compute cost. When traffic arrives, the KEDA HTTP interceptor wakes the agent and forwards the buffered request.

---

## Scope

Applies to EKS only. When the server is running against a non-EKS cluster (Docker Desktop, kind, minikube) scale-to-zero is skipped — no `HTTPScaledObject` is created and ingress targets the backend service directly. The existing EKS mode flag in `internal/k8s/eks.go` is the gate in the spec applier.

---

## Current State

- All workloads run at fixed `replicas: 1` indefinitely.
- KEDA operator and HTTP add-on are installed on the managed cluster (preview + prod).

---

## Phase 1 — K8s: HTTPScaledObject builder

**Package:** `apps/astro-server/internal/k8s`

New file `scaledobject.go`. Builds an `HTTPScaledObject` (`http.keda.sh/v1alpha1`) targeting a Deployment with `minReplicaCount: 0`, `maxReplicaCount: 1`, and `targetPendingRequests: 1`. Takes the target Deployment name and ingress hostname as parameters — reused for both the agent and webhook ingestion Deployments.

**File:** `apps/astro-server/internal/k8s/scaledobject.go` (new)

---

## Phase 2 — Ingress: route through interceptor

**Package:** `apps/astro-server/internal/k8s`

Change `BuildIngress` to target the KEDA interceptor service (`keda-add-ons-http-interceptor`, port `8080` in the `keda` namespace) instead of the backend service. Applies to both the agent ingress and the ingestion webhook ingress — they use separate ALB groups but both route through the same interceptor.

**File:** `apps/astro-server/internal/k8s/ingress.go`

---

## Phase 3 — Spec applier: wire it together

**Package:** `apps/astro-server/internal/k8s`

After applying workloads, call `applyHTTPScaledObject()` for:
- The agent Deployment + its ingress hostname
- Each `webhook` ingestion Deployment + its ingress hostname

Non-webhook ingestion entries (schedule, startup, manual) run as Jobs/CronJobs and are unaffected. StatefulSet dependencies (knowledge, models) remain at `replicas: 1` and stay running.

**File:** `apps/astro-server/internal/k8s/spec_applier.go`

---

## Key Files

| Change | File |
|--------|------|
| `HTTPScaledObject` builder | `apps/astro-server/internal/k8s/scaledobject.go` (new) |
| Ingress → interceptor | `apps/astro-server/internal/k8s/ingress.go` |
| Applier wiring | `apps/astro-server/internal/k8s/spec_applier.go` |
