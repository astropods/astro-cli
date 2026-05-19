## Summary

Deployment HTTP handlers resolve Kubernetes clients from each row's `cluster_id`, matching the worker routing introduced in PR 4. Primary deployments (`cluster_id` null) behave as before; additional clusters use `registry.Get`.

## Design

- **Resolution** — `deploymentClusterClient` in the handlers package maps `deployments.cluster_id` to `registry.Default()` or `registry.Get` (avoids an import cycle if this lived on `k8s.Registry`).
- **Wiring** — `main.go` passes `*k8s.Registry` into deployment-scoped routes only; knowledge, GitHub build logs, probes, and admin gRPC stay on `registry.Default()`.
- **ListDeployments** — each parallel enrichment goroutine resolves its own client from the deployment row (fixes a shared-primary bug).
- **Errors** — missing or disabled additional clusters return 404/503 on single-deployment routes; list falls back to DB-only entries when enrichment cannot resolve a cluster.

## Migration

No operator action for single-cluster installs. Multi-cluster agent parity still requires registered clusters (admin PR 6) and acceptance smoke (PR 8).
