# Staged deployment FGA enforcement

## Summary

PR7.3 turns the PR2–PR7.2 foundation into the first blocking deployment checks. In Preview, reviewed mutations use WorkOS only when an organization explicitly enables Fine-grained access and the deployment's PR4 resource is synchronized. Opted-out organizations, personal deployments, historical deployments, and unconverged resources keep current membership behavior.

Reads remain on legacy authorization until accessible-resource discovery exists. Evaluation routes also remain unchanged because PR6 deferred their resource model rather than assuming they belong to deployments.

## Design

Enforcement has two independent controls: `FGA_ENFORCEMENT_ENABLED` is the operator kill switch, and the server-owned organization experiment is the tenant opt-in. Both must be enabled before a mutation can block. Turning the organization experiment off immediately restores legacy behavior without deleting WorkOS resources or stopping PR5/PR6 shadow comparisons.

Reviewed routes use their PR6 permission before the handler runs:

- `deployment:edit` — rename, avatar, and file writes.
- `deployment:operate` — lifecycle commands, ingestion, and redeploy.
- `deployment:delete` — undeploy.
- `deployment:read` — alert watch/unwatch visibility.

A denial is concealed as not found. Missing membership identity or a WorkOS failure returns service unavailable and never runs the mutation. Astro confirms the session organization owns the deployment before calling WorkOS. The PR7.1 capability API and enforcement share the PR7.2 organization boundary, while shadow checks deliberately keep their broader synchronized-resource gate.

## Review plan

- Follow global enablement and the separate live, shadow, and organization gates in `apps/astro-server/main.go` and `internal/config/config.go`.
- Review blocking behavior in `internal/middleware/deployment_authz.go` and body-addressed redeploy/undeploy in `handlers/deploy.go`.
- Confirm the organization experiment is required for enforcement but never required for shadow observation.
- Confirm tests cover exact actions, hidden denial, retryable failure, opted-out legacy fallback, and body-addressed operations.

## Preview test

1. Deploy a fresh organization deployment and confirm its PR4 WorkOS resource is synchronized.
2. Leave Fine-grained access off. An unassigned member retains current legacy access while PR6 shadow logs continue.
3. Enable Fine-grained access from Organization Settings → Experiments.
4. As an unassigned member, confirm reviewed mutations now conceal the deployment as not found.
5. Grant a reader role. Alert watch/unwatch may pass, but edit, operate, and delete remain denied.
6. Grant roles containing edit, operate, and delete in turn. The matching mutation succeeds on the next request without sign-out; removing the assignment denies the next request.
7. Disable the organization experiment and confirm mutations immediately return to legacy behavior while shadow logs continue.
8. Set `FGA_ENFORCEMENT_ENABLED=false` in a test deployment and confirm it is a global rollback. Production keeps enforcement off.

Any mutation reaching its handler after an FGA denial, cross-organization WorkOS request, enforcement for an opted-out organization, or loss of shadow evidence after opt-out is a release blocker.

## Migration

No organization is opted in automatically and no deployment is backfilled. Configure the five WorkOS permission slugs before Preview testing. Production keeps enforcement off.
