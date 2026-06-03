# Local-mode Launch URL and authn identity

## Summary

Local deployments (`K8sClientMode=local`, e.g. Docker Desktop / k3d) had no Launch URL — there's no ALB ingress to give the agent an external hostname, so the messaging `external_url` was empty and the Launch button stayed hidden. Talking to a local agent required manual `kubectl port-forward`. Authentication didn't work either: the only header-injection point in prod is the ALB OIDC config, which is also absent locally, so messaging fell back to `NoopSessionManager` and treated every request as `anonymous`.

This changes both: local deployments now surface a working `http://localhost:<port>` messaging URL out of the box, and messaging treats every request as the account owner, mimicking the prod ALB → OIDC header flow. Multiple local deployments can run side-by-side; each gets its own NodePort.

## Design

Three coupled changes, in `apps/astro-server` and the `modules/messaging` submodule.

**NodePort + synthetic ingress.** When the deployer detects local mode, the messaging Service is promoted from `ClusterIP` to `NodePort`. The NodePort is left for Kubernetes to auto-allocate from the 30000–32767 range — pinning a single port would limit concurrent local deployments to one. The normalized store seeds a placeholder ingress row (`hostname=localhost`, `tls_enabled=false`); after the Service is created the applier reads back the assigned port and calls `Store.UpdateMessagingIngressHost` to overwrite the row with `localhost:<port>`. `GetMessagingURLs` honors `tls_enabled` so it returns `http://localhost:<port>`, picked up by the Launch button via the existing `external_urls[]` field.

Reaching the agent works because Docker Desktop / k3d publish NodePort services on the host; the browser hits the resolved port directly, no proxy involved.

**Fixed-identity authn.** The messaging submodule learned a new `FixedSessionManager` that returns the same session for every web request, sourced from `WEB_AUTHN_TEST_USER_ID`. The astro-server deployer, in local mode, resolves the deployment's account owner via `GetFirstMemberUserID` and bakes that user ID into the messaging pod env. Effect: messaging authz behaves as if the developer is signed in via OIDC, with no ingress in the path.

Preference order at messaging startup: `FixedSessionManager` (env set) → `HeaderSessionManager` (`ASTRO_AUTHZ_TOKEN` set) → `NoopSessionManager`.

**Pod-reachable authz callback.** The deploy token's `iss` claim carries the URL the messaging pod calls back to authorize identities. In local mode `FrontendURL` defaults to `http://localhost:5173`, which from inside a pod resolves to the pod's own loopback — `localhost`/`127.0.0.1` get rewritten to `host.docker.internal` before signing. The Vite dev server allows that Host header explicitly so its `/api` proxy forwards into astro-server. Browser-facing redirects continue to use the unrewritten URL.

```go
// spec_applier.go — auto-allocated NodePort, read back after apply
svcType := corev1.ServiceTypeClusterIP
if a.localMode && webEnabled {
    svcType = corev1.ServiceTypeNodePort
}
// (no NodePort set; kube-proxy assigns one)
if webEnabled && a.localMode {
    if host, port := a.resolveLocalMessagingHost(ctx, msgSvc.Name); host != "" {
        a.persistMessagingHost(a.deploymentID, host)
        // append ServiceEndpoint with "http://" + host
    }
}
```

## Migration

None. Local-mode behavior change only. Production deployments (`K8sClientMode != "local"`) are unaffected: Service stays `ClusterIP`, ingress is real, identity flows through the existing ALB → header path.

## Known follow-ups

- The admin gRPC `RepairNormalizedSpec` path doesn't carry `LocalMode`, so an admin-driven repair against a local deployment would drop the synthetic ingress row. Re-deploying restores it.
