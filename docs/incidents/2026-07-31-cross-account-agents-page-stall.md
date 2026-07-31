# Incident 1: Cross-account default stalled the Agents page

| Field | Value |
| --- | --- |
| Date | 2026-07-31 |
| Status | Draft; mitigation deployed, production validation incomplete |
| Severity | To be assigned |
| Window based on completed production deployments | 11:41–14:21 EDT (15:41–18:21 UTC) |
| Primary surface | Authenticated Agents page |
| Related changes | [#1726](https://github.com/astropods/astro/pull/1726), [#1728](https://github.com/astropods/astro/pull/1728), [#1823](https://github.com/astropods/astro/pull/1823), [#1824](https://github.com/astropods/astro/pull/1824) |
| Incident owner | TBD |
| Reviewers | TBD |

## Executive summary

The 2026-07-31 production release changed authenticated resource pages from one active-account scope to a page-local multi-account filter. On Agents, an empty filter meant **All accounts**, so ordinary navigation no longer loaded the active account with the existing server-rendered request. It waited for a client-side aggregate query across every account membership, including every required pagination step, before publishing the result.

This made the Agents page appear hung for a user who belonged to many accounts. The reported Postman workspace contained only roughly 50–60 agents; that single account was not an unreasonable workload and had loaded through one bounded request before the change. The regression came from silently expanding the initial scope beyond that account and withholding its usable result until the larger aggregate operation settled.

We first prepared a forward performance fix in #1823, then chose the safer mitigation: #1824 reverted the client behavior introduced by #1726 while retaining the server aggregation API from #1728. Production completed the revert at 14:21 EDT. The selected account is again loaded through a bounded server-rendered request.

No data loss or cross-account authorization failure is known. This was a read-path availability and latency incident. The number of affected users and the final post-mitigation latency measurements remain to be established.

## Customer impact

- The Agents page was extremely slow or appeared to hang for at least one authenticated user with many account memberships and a workspace containing roughly 50–60 agents.
- The page did not show the selected workspace's agents promptly even though that workspace fit in one normal account-scoped response.
- Initial work grew with the user's complete membership set and the number of pages in each account, rather than with the account the user intended to view.
- Agents also loaded observability summary data across accounts, adding work to the critical path.
- Blueprints, Knowledge Stores, and Observability used related shared infrastructure, but no comparable customer-visible slowdown was confirmed on those pages during this incident.
- We have no evidence of corrupted data, lost writes, or unauthorized resource disclosure.

## Detection

The incident was detected through direct production use, not an automated latency or availability alert. The report was specific: the Agents page was hanging for the Postman workspace despite the workspace having only about 50–60 agents.

During mitigation, an `astro-client.http` log recorded `GET /blueprints 500 (58ms)` at 13:48 EDT. Investigation showed that this request:

- occurred while production still ran the pre-revert release commit;
- failed during SSR, before any browser-side aggregate page walk could begin; and
- requires the paired `astro-client.ssr` exception to determine whether it was a real render error or a canceled render recorded as 500.

That log is not evidence that the revert caused a Blueprint failure and is not included in the primary root cause. Its exact exception remains an open follow-up.

## Timeline

All times below are 2026-07-31 EDT unless another date is shown.

| Time | Event |
| --- | --- |
| 2026-07-24 12:33 | #1728 merged, adding bounded server endpoints for aggregating Blueprints, Knowledge Stores, and Deployments across memberships. |
| 2026-07-24 14:28 | #1726 merged, replacing the client active-account scope with page-local multi-account filters and making an empty selection mean All accounts. |
| 11:41 | Production deployment of release commit `1fc92d752` completed. The release included the cross-account list experience. |
| After 11:41 | The Agents page was reported as extremely slow/hung for the Postman workspace. Exact first-observed and incident-declaration times need confirmation. |
| 12:26 | #1823 opened as a forward fix: default to one account, restore the single-account SSR loader, and progressively load explicit aggregate views. |
| 13:19 | #1824 opened to revert the client feature while preserving unrelated changes and the server aggregate API. |
| 13:42 | #1824 merged as `e1eef80df`. Main CI completed successfully at 13:51. |
| 13:47 | Preview deployment of the revert completed. |
| 13:48 | `/blueprints` emitted a 58 ms SSR 500 while production still ran `1fc92d752`; the paired SSR exception was not captured in this investigation. |
| 14:16 | Production deployment of the revert started. |
| 14:21 | Production deployment of `e1eef80df` completed. This is the mitigation time used for the confirmed exposure window. |
| 14:28–14:31 | Post-deploy smoke tests ran: 36 passed, 1 skipped, and 1 failed in the pre-existing saved-variable autofill test. The same test failed before the revert, so it did not validate or invalidate this mitigation. |

## Technical root cause

Before #1726, the initial Agents path was bounded by the active account:

1. The route loader resolved one active account.
2. The server fetched that account's deployments.
3. The loader primed the matching TanStack Query cache.
4. The page rendered the complete account result, including the reported 50–60-agent workspace.

PR #1726 changed both the account-picker semantics and the data-loading boundary:

1. The active-account picker became a page-local multi-select filter.
2. No selected values represented All accounts, so the default scope was the user's complete membership set.
3. The single-account SSR loader was removed from Agents.
4. The browser called the aggregate API and walked all required pages for all selected memberships.
5. The aggregate query withheld its merged result until the complete page walk settled.
6. Agents performed additional account-level observability work before the view was fully useful.

The server endpoint bounded concurrency to six accounts, which protected backend concurrency but did not bound total work or time to first useful content. Total latency still grew with membership count, per-account resource count, slow accounts, later pages, and auxiliary queries. A single slow or high-cardinality membership could hold the entire initial view in a loading state.

The root cause was therefore not that 50–60 cards are intrinsically too many to render. It was making an expanded, complete cross-account catalog a blocking default and removing the proven single-account SSR path.

## Contributing factors

- **Unsafe default semantics.** “No filter” meant All accounts rather than the personal or active account. The most expensive scope required no explicit user action.
- **Correctness was favored over progressive availability.** The catalog walker intentionally withheld partial accounts so search would not present incomplete data. That is internally consistent, but it made time to first content equal time to complete aggregation.
- **Backend bounds were mistaken for a user-facing latency bound.** A six-account concurrency limit limits pressure, not aggregate duration or work.
- **Representative performance coverage was missing.** Tests covered pagination, failures, cache merging, and filter correctness, but not a user with many memberships and 50–60 agents in the intended account.
- **The release changed several pages and account semantics together.** The broad client change increased the rollback surface and made it harder to isolate Agents despite the observed regression being concentrated there.
- **No production SLO guarded the interaction.** We did not alert on Agents time to first content, total settle time, aggregate page count, or account fan-out.
- **The first response attempted a complex forward fix.** #1823 combined defaulting, SSR restoration, progressive aggregation, cache warming, retry behavior, and shared-page cleanup. During an active incident this increased review and CI surface before we chose a smaller revert.
- **Smoke tests were already noisy.** An unrelated saved-variable test failed before and after mitigation, preventing the production smoke workflow from serving as a clear recovery signal.
- **SSR error logs lack immediate correlation.** The HTTP 500 line did not include the render exception or request identifier, which diverted investigation during mitigation.

## Resolution and recovery

#1824 reverted the client feature introduced by #1726 while resolving conflicts in favor of features merged afterward. It restored:

- global active-account selection;
- one-account server-rendered loaders for Agents, Blueprints, and Knowledge Stores;
- bounded account-scoped query priming; and
- account-keyed client caching after hydration.

The server aggregate endpoints from #1728 remain available but are no longer invoked by these pages as their default read path. Keeping the endpoints did not preserve the incident behavior; the regression required the #1726 client to make aggregation the default.

The production deployment completed successfully at 14:21 EDT. Recovery is considered mitigated, not fully verified, until the affected Postman account is measured in production and the account switcher is validated across personal and organization transitions.

## What went well

- The report included a concrete high-value workload: one workspace with approximately 50–60 agents.
- Comparing the page before and after #1726 quickly exposed the removed SSR loader and expanded default scope.
- The revert targeted the client feature rather than removing the independently bounded server API.
- Conflict resolution preserved unrelated features added after #1726.
- Main CI, including all four client E2E shards, passed before production deployment.
- Deployment timing let us establish that the later Blueprint SSR 500 occurred before the revert reached production.

## What went poorly

- A fundamental scope and loading-model change shipped without a representative enterprise-account performance test.
- Default navigation performed the maximum available read instead of the minimum useful read.
- The page showed a blocking loading experience instead of the selected account as soon as it was available.
- We started with a broad optimization PR during the incident before reducing the response to a feature revert.
- Automated production validation was red for an unrelated test both before and after mitigation.
- We cannot yet quantify affected users, p95/p99 latency, request fan-out, or the exact incident declaration time.

## Corrective and preventive actions

| Priority | Action | Owner | Status |
| --- | --- | --- | --- |
| P0 | Validate the reverted Agents page against the affected Postman workspace. Record TTFB, time to first cards, total settle time, request count, and complete rendering of all 50–60 agents. | TBD | Open |
| P0 | Add an E2E/performance fixture with many memberships and 60 agents in one target account. Assert default navigation issues one bounded account request and does not call aggregate routes. | TBD | Open |
| P0 | Make organization switching atomic before further account-switcher work: switch the WorkOS organization session first, then commit the active-account cookie and revalidate; preserve the previous account on failure. | TBD | Open |
| P0 | Close or explicitly supersede #1823 so its incident-era implementation cannot be merged accidentally after #1824. Preserve useful design notes separately. | TBD | Open |
| P1 | Define client performance budgets for authenticated list pages: SSR TTFB, time to first content, time to interactive, and maximum initial account/API fan-out. | TBD | Open |
| P1 | Add telemetry for selected scope size, aggregate page count, failed account count, and first-content/settled durations without logging account names. | TBD | Open |
| P1 | Require All accounts and multi-account views to be explicit. If reintroduced, render progressively or server-render the first useful page and keep failed/slow accounts out of the blocking path. | TBD | Open |
| P1 | Add a release/canary check using a high-cardinality internal account before broad production rollout of list-loading or account-scope changes. | TBD | Open |
| P1 | Fix the saved-variable production smoke test so the smoke workflow is a trustworthy deployment gate. | TBD | Open |
| P1 | Correlate HTTP request logs with SSR exception logs and classify client-disconnected render aborts separately from real 500s. | TBD | Open |
| P2 | Retain #1728 as an opt-in API and benchmark it independently before assigning it to a default page path. Add server-side caching only against measured bottlenecks. | TBD | Open |

## Open questions and evidence gaps

- How many users navigated to Agents during the confirmed exposure window, and how many had more than one membership?
- When during the initial rollout did affected traffic first reach the new client, before the deployment completed at 11:41 EDT?
- What were p50, p95, and p99 time to first content and total settle time before and after the revert?
- What were the largest membership count, aggregate page count, and deployment count observed?
- Did any user receive a hard request failure, or was the primary symptom indefinite/very long loading?
- What is the paired `astro-client.ssr` exception for the 13:48 `/blueprints` 500?
- Has the affected Postman workspace been manually re-tested after the 14:21 production deployment?
- What severity and formal incident start time should be assigned?

## Evidence reviewed

- Client behavior and changelog from #1726 (`9844b1d11`).
- Server aggregate API and changelog from #1728 (`ac9798cc7`).
- Forward-fix diagnosis and passing CI from #1823.
- Revert design, conflict resolution, CI, and merge from #1824 (`e1eef80df`).
- Production, preview, and smoke workflow timestamps from GitHub Actions.
- Production symptom reports and decisions captured during the incident response conversation.
- `astro-client` request logging, SSR error handling, route loaders, and account-scoped query paths.
