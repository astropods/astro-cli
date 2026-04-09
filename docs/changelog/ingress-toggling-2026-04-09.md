# Messaging Ingress Toggling / Disappearing on Every Deploy

## Summary

On every deploy, the messaging web ingress (`sasbot-ingress-messaging` and equivalents) either
flaps between pointing to the wrong backend or disappears entirely and is never recreated.
Confirmed live against `sasbot` in the preview managed cluster: ingress was deleted at 17:06:32
during a deploy and was still missing at 17:08:57 when the reconciler flagged `missing: 1`.

Two bugs are compounding:

1. **Finalizer race in stale cleanup** — the primary cause of the ingress disappearing permanently
2. **Same hostname collision** — the cause of ALB rule toggling when both ingresses coexist

---

## Root Cause 1 — Finalizer race (ingress permanently gone)

### What happens

`cleanupStaleBuildResources` runs at the start of every deploy and deletes any resource whose
`app.kubernetes.io/version` label differs from the current buildID. Ingress names are stable
across builds (e.g. `sasbot-ingress-messaging`), but their labels carry the buildID, so the old
ingress is always stale relative to the new build.

```
[cleanup] deleting stale Ingress sasbot-ingress-messaging (build 6c58a8a6)
```

After the delete call returns, `applyIngress` immediately tries to Create the replacement.

The AWS Load Balancer Controller puts a **finalizer** (`group.ingress.k8s.aws/...`) on every
ingress it provisions. When the DELETE request arrives, K8s sets `deletionTimestamp` on the
object but cannot remove it until the ALB controller deprovisions the listener rule and strips
the finalizer. During this window the object still exists in the API.

`applyIngress` → Create → `AlreadyExists` → Get + Update

The Get returns the object with `deletionTimestamp` set. The Update is accepted by K8s, but
the ALB controller treats the object as terminating and ignores the new content. It finishes
removing the listener rule, strips the finalizer, and the object is garbage-collected. The
deploy reports `status: updated` with no error. The ingress is gone.

### Why stale cleanup runs at all

`cleanupStaleBuildResources` was designed to handle resources whose **names** change between
builds — Secrets and ConfigMaps include the buildID in their name
(`sasbot-6c58a8a6-credentials` → `sasbot-42327e2c-credentials`), so old objects are genuinely
orphaned. Applying the same label-based purge to stable-named resources (Deployments, Services,
Ingresses) is incorrect: those resources should be updated in-place by their respective
`apply*` functions, not deleted and recreated.

### Why Services and Deployments don't exhibit the same symptom

Services and Deployments are also deleted and recreated by stale cleanup. They survive because:
- Deployments have no ALB finalizer; the old ReplicaSet keeps pods running during the gap.
- Services have no finalizer; deletion and recreation is near-instant.

Ingresses are uniquely vulnerable because the ALB finalizer creates a multi-second (sometimes
multi-minute) window during which the replacement create/update is silently absorbed into the
terminating object.

---

## Root Cause 2 — Same hostname collision (ALB rule toggling)

When an agent spec has both an exposed agent endpoint **and** `interfaces.adapters: [web]`, two
separate ingresses are created. Both fall back to the identical hostname formula when no
explicit `expose.domain` is configured:

```go
// agent ingress (spec_applier.go ~line 159)
host = GenerateIngressHost(agentName, a.namespace, a.ingressDomain)

// messaging ingress (spec_applier.go ~line 554) — identical call
host = GenerateIngressHost(agentName, a.namespace, a.ingressDomain)
```

Both ingresses are placed in the same ALB group (`albGroupName`). The ALB controller sees two
ingresses with identical `host + path /` rules pointing to different target groups. On each
reconciliation cycle, whichever ingress the controller processes last "wins" the listener rule.
The result is non-deterministic ALB routing that alternates between the agent backend and the
messaging backend.

---

## Fix

### Fix 1 — Move stale cleanup to after the apply phase

`cleanupStaleBuildResources` currently runs at the **start** of `ApplyDeploymentSpec`, before
any resources are created. Moving it to the **end** (after all `apply*` calls) eliminates the
finalizer race without changing any of the cleanup logic.

Why it works: `applyIngress` (and every other `apply*` function) does a Get + Update when a
resource already exists. The Update writes the **new buildID** into the resource's labels. By
the time stale cleanup runs, the ingress already carries the current buildID and the label
check `version != currentBuildID` is false — cleanup skips it.

The rename case still works correctly: when a resource is renamed between builds (e.g. a tool
`foo` → `bar`), `apply` creates the new `bar` resource, then cleanup finds the old `foo`
resource (still labelled with the old buildID) and deletes it.

```go
// In ApplyDeploymentSpec, move this block from the top to the bottom,
// just before the orphan cleanup call:

// Clean up resources from previous builds whose names may have changed
if cleanupErrs := a.cleanupStaleBuildResources(...); ...
```

No changes to the cleanup logic itself are needed.

### Fix 2 — Give the messaging ingress a distinct hostname

Add `GenerateMessagingIngressHost` to `ingress.go` that incorporates a `"chat"` component in
the hash input, producing a hostname that is unique from the agent ingress:

```go
func GenerateMessagingIngressHost(agentName, namespace, domain string) string {
    // Reuse GenerateIngestionIngressHost logic with "chat" as the component name
    return GenerateIngestionIngressHost(agentName, namespace, "chat", domain)
}
```

Replace the fallback in `spec_applier.go`:

```go
// Before
host = GenerateIngressHost(agentName, a.namespace, a.ingressDomain)

// After
host = GenerateMessagingIngressHost(agentName, a.namespace, a.ingressDomain)
```

The hash now includes `"chat"` so the resulting hostname differs from the agent ingress even
when both use the server-default domain.

### Fix interaction

Fix 1 alone stops the permanent disappearance. Fix 2 alone stops the ALB rule toggling when
both ingresses coexist. Both are needed: Fix 1 is the higher-severity issue (ingress gone after
every deploy), Fix 2 is the correctness issue (wrong backend served when both are live).

Note that Fix 1 also naturally resolves any unnecessary churn on Services and Deployments that
stale cleanup was triggering — those resources will now be updated in-place rather than
deleted and recreated on every deploy.

---

## Migration

Existing deployments with the messaging ingress will have their ingress hostname changed by
Fix 2. The old hostname will be cleaned up by orphan cleanup on the next deploy. External DNS
will pick up the new hostname automatically. There is a brief (~TTL) window during which the
old hostname no longer resolves; users with saved links to the old messaging URL will need to
use the new one.

No action required for deployments without `interfaces.adapters: [web]`.
