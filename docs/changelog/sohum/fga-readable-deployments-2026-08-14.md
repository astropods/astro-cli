# FGA-readable deployments

## Summary

Organization membership no longer implies visibility of every new FGA-managed deployment. When the global FGA switch and the organization's Fine-grained access experiment are enabled, a member must have `deployment:read` to see or open a deployment.

## Design

Deployment lists use WorkOS resource discovery once per selected organization, then filter Astro's database query with the returned deployment IDs. Account-specific list and count requests discover only that account; cross-account surfaces discover each selected organization. Successful discovery results are cached in-process for three seconds per membership and organization, collapsing list polling bursts while bounding role-change latency. The experiment gate is still checked on every request, and changing it explicitly invalidates the organization's cache; changing the global switch reconstructs discovery on restart. The fan-out has a two-second request budget: ordinary failures conceal only the affected organization, while a deadline returns `503` rather than hanging or returning partial data.

The same boundary now protects deployment detail and all cataloged control-plane reads, including status, runtime, files, logs, observability, configuration, network, and cached summaries. A denied read is concealed as `404`. During a multi-organization list, a WorkOS or membership failure hides only the affected organization's managed deployments; healthy and legacy organizations still render. Request-wide discovery failures return `503`.

Rollout remains incremental:

- Personal accounts, organizations with the experiment off, and environments with the global enforcement switch off retain current behavior.
- Synchronize every current organization member into `account_member_workos` before enabling the organization experiment; a missing WorkOS membership fails closed. The global switch may be deployed earlier while the experiment remains off.
- Historical deployments without a PR4 lifecycle row retain current behavior until backfill.
- Once a deployment enters the lifecycle ledger, incomplete synchronization fails closed until WorkOS converges.
- WorkOS resource registration and the creator's `deployment-admin` assignment are sequential, not atomic. The deployment remains fail-closed between those calls rather than introducing a local creator bypass.
- Evaluation-model routes and chat/messaging data-plane routes keep their separately documented authorization ownership.

## Review

- Confirm list discovery makes at most one WorkOS request per selected membership and opted-in organization during a three-second polling burst, and experiment changes invalidate that cache. Review `apps/astro-server/internal/authz/deployment_discovery.go`, `apps/astro-server/handlers/experiments.go`, and their adjacent tests.
- Confirm every deployment list, count, summary, detail, and cataloged control-plane read applies `deployment:read` when FGA is active. Review `apps/astro-server/handlers/deploy.go`, `apps/astro-server/handlers/user_deployments.go`, `apps/astro-server/handlers/user_deployment_summaries.go`, `apps/astro-server/handlers/user_resources.go`, `apps/astro-server/internal/middleware/deployment_authz.go`, and `apps/astro-server/main.go`.
- Confirm the list cache changes with the live authorization result and does not cache an authorization decision. Review `apps/astro-server/handlers/user_deployments.go`, `apps/astro-server/handlers/user_resources.go`, and `apps/astro-server/handlers/user_resources_test.go`.
- Confirm denials return `404`, per-organization discovery failures fail closed only for that organization, request-wide failures return `503`, and opted-out, personal, and historical resources stay on legacy behavior. Review `apps/astro-server/handlers/deployment_visibility.go`, `apps/astro-server/internal/authz/deployment_account_resolver.go`, `apps/astro-server/internal/authz/deployment_discovery.go`, `apps/astro-server/internal/middleware/deployment_authz.go`, and their adjacent tests.

## Preview test

1. Enable the organization's Fine-grained access experiment with global enforcement enabled.
2. Create a deployment as Sohum and verify its `deployment-admin` assignment can see the card, open it, and load its control-plane tabs.
3. Sign in as Jessye with no deployment role. Verify the deployment is absent from deployment lists and summaries, and its direct URL returns `404`.
4. Assign Jessye `deployment-viewer`. Refresh and verify the card and read-only deployment surfaces appear.
5. Remove that role. Refresh and verify they disappear after the discovery cache's maximum three-second window, including after a previously cached list response.
6. Turn the organization experiment off and verify current organization-member behavior returns.

## Migration

No database migration or new WorkOS object is required. The configured Viewer, Builder, and Admin deployment roles must continue to include `deployment:read` wherever read access is intended.
