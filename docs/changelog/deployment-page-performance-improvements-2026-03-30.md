# Deployment detail page: single-deployment endpoint (step 1)

## Summary

The deployment detail page was calling `GET /api/v1/deployments?account=...`, which fetches every deployment for the account and discards all but one. On the server, each deployment triggers 5 sequential Kubernetes API calls, so an account with 10 deployments made 50 K8s round-trips to render a single detail page. This adds a dedicated `GET /api/v1/deployments/:id` endpoint so the detail page fetches only what it needs.

## Design

The new server handler (`GetDeployment`) mirrors `ListDeployments` but operates on a single DB row. It uses the existing `resolveDeployment` helper (shared with `StopDeployment`, `WakeUpDeployment`, `GetDeploymentLogs`) which fetches the deployment by ID and verifies the caller is a member of the deployment's account — no `?account=` query parameter needed. It then runs the same 5 K8s calls scoped to that deployment's namespace only. If the namespace does not exist, it falls back to a DB-only response, matching the existing list behavior.

On the client, a `useDeployment(id)` query hook wraps the new endpoint. It uses its own query key (`deploymentKeys.detail(id)`) so it is fully independent of the list cache. The transitional-state polling (`refetchInterval: 3000ms` for pending/deploying/undeploying states) is preserved. `DeployedAgentDetail` now calls `useDeployment` instead of `useDeployments` + `.find()`.

`useDeployments` is unchanged and continues to serve the agent list page and sidebar.

## Migration

No action required. The client change is transparent — the detail page now hits the new endpoint automatically.
