## Summary

The blueprint detail page wants to surface every agent the viewer has deployed from that blueprint, regardless of which of their accounts the deployment lives in. The existing `GET /api/v1/deployments` required `?account=` and returned only that one account's deployments, forcing the client to fan out N requests across the viewer's accounts.

## Design

The `account` query parameter on `GET /api/v1/deployments` is now optional. When omitted, the handler resolves the authenticated user's accounts via `accountStore.GetAccountsForUser` and aggregates deployments from each, returning the same `{deployments, count}` envelope.

The per-account enrichment pipeline (messaging URLs, audit timestamps, avatars, latest build IDs) was extracted into a private `enrichDeploymentsForAccount` helper so the per-account and cross-account paths share one code path and cannot drift. The deploy cache stays per-account; the cross-account path skips it. A failed load for a single account is logged and skipped rather than blanking the whole response — a single bad account shouldn't take down the sidebar.

Per-account enrichment in the cross-account path runs concurrently via `errgroup` with a concurrency cap, since each account's load is independent. This keeps latency bounded by the slowest single account rather than the sum across all of them.

Each `AgentDeploymentSummary` now carries `account_id` and `account_name` so the consuming sidebar can attribute, link, and route per deployment without joining against a second endpoint. The single-account path stamps the same fields for shape consistency.

An optional `?build_id=` filter (comma-separated or repeated) restricts the response to deployments matching a set of build IDs. The blueprint sidebar uses this to ask for "deployments of this blueprint" directly instead of pulling every deployment across every account and filtering client-side. The filter pushes into SQL via a build-filtered variant of `GetVisibleDeploymentsByAccount`, so enrichment only runs on rows the caller wants. The cache stays unfiltered-only: a filtered request bypasses both the read and the write to avoid blowing up the per-account key space.

`build_id` is also required when `account` is omitted. The cross-account path exists for the blueprint sidebar, which always knows the blueprint's builds. An unfiltered cross-account fan-out would return every deployment in every account the user belongs to in a single uncached response, so the handler rejects that combination with `400` rather than silently truncating or capping. The unfiltered single-account path is unchanged. If a future caller legitimately needs the cross-account firehose, the right answer is cursor pagination, not a defensive cap.

The `build_id` filter itself is capped at 200 values per request — well above realistic blueprint version counts, but enough to keep a misbehaving caller from expanding the SQL `ANY()` array into something that strains the query planner. Requests over the cap are rejected with `400` (same loud-failure pattern: no silent truncation).

When `account` is supplied, behavior is unchanged: same authorization (`IsMember`), same cache, same payload shape (now with the account fields included).

## Migration

No action required. Existing callers passing `?account=` see no behavior change. New callers can drop the parameter to receive the aggregated list.
