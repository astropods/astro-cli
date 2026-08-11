# Deployment FGA shadow decisions

## Summary

Connect Astro's request path to live WorkOS FGA checks without changing access yet. For new FGA-ready deployments in Preview, Astro compares today's account-membership result with `deployment:read` or `deployment:edit`, records whether the answers agree, and always returns today's result.

## Design

`FGA_SHADOW_ENABLED` turns observation on for an environment. Within that environment, a deployment is eligible only when its PR4 reconciliation row shows a converged registered WorkOS resource. This limits calls to new, known-good organization deployments without hard-coding tenant IDs. Eligibility is resolved before the legacy membership comparison, so personal, historical, and pending resources skip both the comparison and WorkOS.

The WorkOS call and its allow/deny answer are real. It is a shadow check only because that answer is not authoritative yet: the existing membership checker still controls the HTTP response, while Astro records whether the two systems agree. A later enforcement PR will make the FGA answer control access; PR5 intentionally provides no switch that can turn a shadow denial into a `403`.

Only two routes are sampled in this PR:

| Request | Shadow action | Why this route |
| --- | --- | --- |
| `GET /api/v1/deployments/:id` | `deployment:read` | Proves resource visibility with one bounded detail read. |
| `PATCH /api/v1/deployments/:id` | `deployment:edit` | Proves a simple mutation through the rename flow already synchronized by PR4. |

Every other deployment route remains out of scope until PR6's hand-reviewed action inventory. In particular, this PR does not infer policy for lists, logs, runtime state, configuration, secrets, observability, chat/data-plane traffic, redeploy, restart, or undeploy.

For an eligible request, the existing membership checker remains authoritative while the FGA checker uses the session's WorkOS organization membership ID. A missing membership ID is a repairable identity condition, not a denial; refresh, switch-org, or re-login can repair it. The gate and both checkers share one request-cached deployment/account lookup; the membership query remains separate. Permission decisions are never cached, so role changes take effect on the next request.

Shadow comparison runs outside the request goroutine with a detached two-second timeout that bounds deployment resolution, membership lookup, and the WorkOS call. Each API process allows at most 16 shadow evaluations at once; a request that finds every slot occupied skips its comparison immediately and writes one debug log, rather than waiting or creating another goroutine. A panic in background work is recovered and logged instead of terminating the API process. The observer reuses the single WorkOS FGA client created at server startup and is disabled when the API key is absent. Expected probes for missing deployments log at debug; unexpected resolution failures remain warnings.

PR5 intentionally does not cache permission answers or sample eligible requests. The client does not interval-poll `GET /api/v1/deployments/:id`; its three-second transition polling uses list and status routes that are outside this shadow map. A detail mount or refresh and a rename therefore make one check each. That Preview-only volume is well below WorkOS's [general limit of 6,000 requests per minute per API key](https://workos.com/docs/reference/rate-limits), while checking every eligible request gives complete comparison evidence and makes role changes visible immediately. `429` failures remain observable in the existing failure log and are the signal to revisit sampling as PR6 expands route coverage.

The structured outcomes are intentionally visible during Preview testing:

| Log | Meaning | HTTP behavior |
| --- | --- | --- |
| `FGA shadow decision matched` | Membership and WorkOS agree. | Existing response is returned. |
| `FGA shadow decision mismatch` | The future FGA boundary would change the result. | Existing response is still returned. |
| `FGA shadow check failed` | WorkOS or resource resolution failed. | Existing response is still returned. |
| `FGA shadow check skipped` | The resource is personal or its PR4 registration is not converged. | No WorkOS decision is attempted. |
| `FGA shadow check skipped: concurrency limit reached` | All 16 per-process shadow slots are occupied; each log counts one dropped comparison. | No WorkOS decision is attempted and the existing response continues immediately. |
| `FGA shadow check panic recovered` | Detached shadow work panicked and was contained. | Existing response is unaffected; the API process continues. |

### Review plan

1. Start with the rollout switch. Preview should turn shadow mode on, but Astro should call WorkOS only when the API key exists and PR4 finished registering that organization deployment. Personal, older, and still-syncing deployments should skip WorkOS. Read `config/astro-server/preview.env`, `apps/astro-server/main.go`, `apps/astro-server/internal/authz/fga_checker.go`, and `apps/astro-server/internal/authz/deployment_account_resolver.go`.
2. Make sure nothing in this PR can change access. Astro should compare today's membership answer with WorkOS, log what happened, and still preserve today's answer when WorkOS disagrees, fails, or cannot check. Read `apps/astro-server/internal/authz/membership_checker.go` and `apps/astro-server/internal/authz/shadow_checker.go`.
3. Check the small route list. It should contain only deployment detail GET as `deployment:read` and rename PATCH as `deployment:edit`; every other route should be ignored for now. Read `apps/astro-server/internal/middleware/deployment_authz.go`.
4. Follow one check into WorkOS. The organization session should provide the membership ID, the URL should provide the deployment ID, and the route map should provide the permission. Those exact values should reach the existing WorkOS client. Follow `apps/astro-server/internal/middleware/authz_subject.go` to `apps/astro-server/internal/authz/fga_checker.go`, then `apps/astro-server/internal/authz/workos_fga.go`.
5. Check what is cached. Astro may reuse the deployment-to-account database lookup inside one comparison, but it must never cache the WorkOS allow/deny answer. Read `apps/astro-server/internal/authz/request_cache.go` and `apps/astro-server/internal/authz/deployment_account_resolver.go`.
6. Make sure shadow work cannot hurt the real request. The response should continue immediately; background checks should stop after two seconds, never exceed 16 at once per API process, recover panics, and reuse the one WorkOS client created at startup. Read `apps/astro-server/internal/middleware/deployment_authz.go`, `apps/astro-server/main.go`, and `apps/astro-server/deps.go`.
7. Check expected request volume and failure visibility. The deployment detail query should not poll on an interval, role changes should be checked live on the next request, and WorkOS rate-limit errors should appear in the failure logs. Read `apps/astro-client/src/api/queries/deployments.ts` and `apps/astro-server/internal/authz/shadow_checker.go`.
8. Finish with the tests. They should cover eligible and skipped deployments, personal accounts, missing membership IDs, missing deployments, matches, mismatches, WorkOS failures, excluded routes, non-blocking requests, panic recovery, and the concurrency limit. Read `apps/astro-server/internal/authz/deployment_account_resolver_test.go`, `apps/astro-server/internal/authz/fga_checker_test.go`, `apps/astro-server/internal/authz/shadow_checker_test.go`, and `apps/astro-server/internal/middleware/deployment_authz_test.go`.

### Preview testing plan

Before testing, keep the WorkOS `member` role without deployment permissions. `deployment-reader` must contain `deployment:read`, and `deployment-editor` must contain both `deployment:read` and `deployment:edit`. Sohum and Jessye must each switch into the same Preview organization; Jessye should re-login first if her session does not yet contain her WorkOS membership ID.

1. Confirm shadow mode is enabled in Preview:

   ```bash
   kubectl get configmap astro-server-config -n astro-server \
     -o jsonpath='{.data.FGA_SHADOW_ENABLED}{"\n"}'
   ```

   The result must be `true`.

2. As Sohum, deploy a new agent into the organization through the Preview UI. Confirm WorkOS contains a deployment resource with that deployment ID and gives Sohum `deployment-editor`. This new resource is what makes the request eligible for a shadow check.

3. Watch every Preview API replica for shadow results:

   ```bash
   kubectl logs -n astro-server -l app=astro-server -c astro-server \
     --since=15m --prefix=true --max-log-requests=10 -f \
     | rg --line-buffered 'FGA shadow'
   ```

4. As Sohum, open the deployment and rename it in the Preview UI. Both actions must succeed. The logs should say `FGA shadow decision matched` for `deployment:read` and `deployment:edit` because Sohum has access through the organization admin role and the direct editor assignment.

5. As Jessye, with no deployment role, open and rename the same deployment. Both actions still succeed because PR5 does not enforce the WorkOS answer. Each request should log `FGA shadow decision mismatch` with `membership_allowed=true` and `fga_allowed=false`. This proves WorkOS would deny Jessye while today's membership rule still allows her.

6. Assign Jessye `deployment-reader` on that deployment in WorkOS, then reload it in Preview. No Astro re-login is required for a role change. Reading should now log a match with `fga_allowed=true`; renaming should still log a mismatch with `fga_allowed=false`.

7. Replace Jessye's reader assignment with `deployment-editor` and repeat. Opening and renaming should both log matches with `fga_allowed=true`.

8. Remove Jessye's assignment and undeploy the agent. PR4 should remove the WorkOS resource and its assignments. Personal deployments and older organization deployments that were never registered by PR4 should continue on the existing path without a WorkOS decision.

The shadow log is written shortly after the UI response because the WorkOS call runs in the background. At every step, Astro's visible behavior must remain the same as it was before PR5. Any `403` or other new denial is a regression; real enforcement is deliberately deferred to PR7 and later read-discovery work.

## Migration

No database or client migration is required. Configure the WorkOS permissions and roles above. Preview enables shadow observation for converged new organization deployments; production remains disabled. Historical deployments are intentionally excluded rather than backfilled in this milestone.
