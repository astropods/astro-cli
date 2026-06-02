# Local-mode Launch URL and authn identity

## Summary

Local deployments (`K8sClientMode=local`, e.g. Docker Desktop / k3d) had no Launch URL — there's no ALB ingress to give the agent an external hostname, so the messaging `external_url` was empty and the Launch button stayed hidden. Talking to a local agent required manual `kubectl port-forward`. Authentication didn't work either: the only header-injection point in prod is the ALB OIDC config, which is also absent locally, so messaging fell back to `NoopSessionManager` and treated every request as `anonymous`.

This changes both: local deployments now surface `http://localhost:30100` as the messaging URL out of the box, and messaging treats every request as the account owner, mimicking the prod ALB → OIDC header flow.

## Design

Two coupled changes, in `apps/astro-server` and the `modules/messaging` submodule.

**NodePort + synthetic ingress.** When the deployer detects local mode, the messaging Service is promoted from `ClusterIP` to `NodePort` on a fixed port (`LocalMessagingNodePort = 30100`). The normalized store writes a synthetic ingress row pointing at `localhost:30100` with `tls_enabled=false`, and `GetMessagingURLs` learned to honor `tls_enabled` (was hardcoded `https://`) so it returns `http://localhost:30100`. The Launch button picks that up unchanged from the existing `external_urls[]` field — no client-side changes.

Reaching the agent works because Docker Desktop / k3d publish NodePort services on the host; the browser hits `localhost:30100` directly, no proxy involved.

**Fixed-identity authn.** The messaging submodule learned a new `FixedSessionManager` that returns the same session for every web request, sourced from `WEB_AUTHN_TEST_USER_ID`. The astro-server deployer, in local mode, resolves the deployment's account owner via `GetFirstMemberUserID` and bakes that user ID into the messaging pod env. Effect: messaging authz behaves as if the developer is signed in via OIDC, with no ingress in the path.

Preference order at messaging startup: `FixedSessionManager` (env set) → `HeaderSessionManager` (`ASTRO_AUTHZ_TOKEN` set) → `NoopSessionManager`.

```go
// spec_applier.go — Service promotion
svcType := corev1.ServiceTypeClusterIP
if a.localMode && webEnabled {
    svcType = corev1.ServiceTypeNodePort
}
// ...
if a.localMode {
    httpPort.NodePort = LocalMessagingNodePort
}
```

```go
// normalized.go — synthetic ingress for local
} else if nsCfg != nil && nsCfg.LocalMode {
    host = k8s.LocalMessagingHost()
    tlsEnabled = false
}
```

## Migration

None. Local-mode behavior change only. Production deployments (`K8sClientMode != "local"`) are unaffected: Service stays `ClusterIP`, ingress is real, identity flows through the existing ALB → header path.

Multi-agent locally is still single-tenant — NodePort `30100` is shared across deployments, so only one local agent can be reached this way at a time.

## Known follow-ups

- The admin gRPC `RepairNormalizedSpec` path doesn't carry `LocalMode`, so an admin-driven repair against a local deployment would drop the synthetic ingress row. Re-deploying restores it.
