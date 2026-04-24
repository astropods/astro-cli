# Queen Governance Features: Owner Email, Pause/Unpause, Blueprints Rename

## Summary

Adds governance capabilities to the Queen admin TUI so operators can quickly identify deployment owners and pause deployments instead of deleting them. Also renames the "Agents" page to "Blueprints" to match current product terminology, and fixes a crash on the agents/blueprints page caused by missing build IDs.

## Design

### Owner email on deployments

The `ListAllWithAccount` SQL query is extended with a subquery that resolves the first `account_members.user_id` for each deployment's account (the account creator). The admin gRPC server batch-resolves these user IDs to emails via WorkOS using a new `resolveEmails` helper that deduplicates by user ID and gracefully degrades on failure. A `SetWorkOSClient` setter injects the dependency post-construction in `main.go`, following the existing pattern for `SetHTTPHandler` and `SetWorkOSClientID`.

The `AdminDeployment` proto message gains an `owner_email` field. The Queen web UI adds an "Owner" column to the deployment list (searchable) and an "Owner" info card on the detail page.

### Pause/unpause deployments

The backend `StopDeployment` RPC already scales workloads to zero without deleting resources, but it only accepted `namespace` as a lookup key. The `StopDeploymentRequest` now also accepts `deployment_id`, with the server falling back to namespace if deployment_id is empty (backward-compatible).

Queen's Go proxy gains a `POST /api/admin/deployments/{id}/stop` route. The web UI adds:
- A **Pause** button on the detail page (visible when status is `active`), with confirmation dialog
- The **Wake Up** button now correctly handles both `scaled_down` and `stopped` statuses (previously only `scaled_down`)
- A `stopped` status banner, filter option, and badge color
- **Bulk Pause** and **Bulk Wake Up** actions in the deployment list

### Blueprints rename

The Queen "Agents" page is renamed to "Blueprints" throughout: page component, route (`/admin/blueprints`), sidebar label, TypeScript types (`AdminBlueprint`, `ListBlueprintsResponse`), query hooks (`useBlueprints`, `useBlueprintBuilds`), and query keys. The underlying API endpoints remain unchanged since they are served by the backend.

### truncateUUID crash fix

`truncateUUID` in `lib/utils.ts` crashed when called with `undefined` (e.g. an agent with no `latest_build_id`). Added a nil guard.

## Migration

No database migrations required. The proto changes are additive (new fields). The Queen route changes from `/admin/agents` to `/admin/blueprints` — update any bookmarks.
