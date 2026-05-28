# Deployed agent card v2

## Summary

Redesigns the deployed-agent card and rebuilds the agents page around a fast read-through cache so the grid loads in well under a second. The card reframes the tile around the agent/space theme — avatar-tinted gradient, seeded starfield, nebula sprite tinted by the agent's palette — and surfaces a per-deployment request/token sparkline that previously didn't exist on this page.

## Design

### Card

Vertically stacked: avatar + name/status at top, sparkline centered in a `flex-1` middle band, action row pinned at the bottom. The stretchy middle keeps the action row aligned across cards in the same grid row when peers carry status pills.

- **Tint**: `color-mix` of the avatar palette into `var(--card)`, fading out by 60% height. Same pattern `BlueprintCard` uses.
- **Starfield**: SVG, ~150 stars seeded by `deploymentId`, parallax + twinkle driven by the Web Animations API with a hover-driven `playbackRate` ramp.
- **Nebula**: pre-rendered PNG sprite used as an alpha mask over a `<rect>` filled with the agent's glow color, blended `screen`.
- **Sparkline**: `d3-shape` curveBasis line for requests + a dashed token overlay (independently normalized). Cursor-following tooltip rendered into `document.body` via `createPortal` so the card's `overflow-hidden` doesn't clip it.
- **Status pills**: optional Error + "Update available". When `latestBuildId` is supplied, the pill links to `deploymentConfigurePath(...)?build=<id>`. `StatusBadge` gains a `primary` color for this.

### Sub-second agents page

The page used to block on per-deployment Langfuse fan-out for the sparkline data and per-deployment K8s queries inside `listDeployments`. Both move off the request path:

- **`internal/obssummary`** — per-deployment cache at `obs:summary:<id>` populated by a River periodic worker (`ObsSummaryRefreshWorker`, 10m interval, `RunOnStart`). The summaries handler stops calling Langfuse on the request path and serves Redis reads only; missing entries surface as a zeroed sparkline.
- **`internal/deploycache`** — JSON envelope of `ListDeployments` per account at `dep:agent:<account_id>`. Read-through with 1h SafetyTTL backstop; on hit the handler returns bytes via `c.Data` with zero re-marshal.

Invalidation is **explicit and event-driven**; the TTL is a safety net only. Busts fire from every write site that affects the cached payload: deploy/undeploy/reconcile workers, rollback, display-name + avatar updates, and publish events. For publish/transfer, a new `deploymentstore.ListAccountIDsWithLineageAgent` query fans out invalidation to every downstream consumer whose lineage matches the changed agent, so the "Update available" pill appears immediately instead of waiting on TTL.

Both caches are nil-safe — when `REDIS_URL` is unset, both packages no-op and the handlers fall back to live queries.

### Other agents-page changes

`DashboardStats` (the metric tiles row) is gone; the page is now just the agent grid. Loader drops the slow account-level observability summary. Grid scales 1 → 5 columns with progressive breakpoints.

### Manual cache invalidation (queen)

Two admin RPCs let an operator clear the new caches without waiting on SafetyTTL — useful when something goes systemically wrong and pills/sparklines need to reset immediately.

- `AdminService.InvalidateAccountCaches(account_id)` — busts the agents-page deploy envelope plus every active deployment's obs summary for one account. Wired in queen's Accounts table as a per-row trash icon.
- `AdminService.InvalidateAllCaches()` — failsafe. Iterates every account + every active deployment. Surfaced in queen's Accounts page header behind a two-click arm/confirm. Audit-logged as `cache.invalidate_all`.

Both RPCs are nil-safe; with `REDIS_URL` unset they short-circuit and report zero counts.

## Migration

- Production needs `REDIS_URL` for caching to take effect; without it the handlers run live queries (pre-cache behavior).
- `StatusBadge` gains a `primary` color variant; existing usages unaffected.
- New `astro-client` deps: `d3-shape` + `@types/d3-shape`.
