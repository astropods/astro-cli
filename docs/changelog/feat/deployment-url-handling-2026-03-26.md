# URL-driven tabs and observability cache improvements

## Summary

The deployment detail page had broken tab navigation (no back button support, deep links not working, effects fighting over state) and observability data refetched from scratch every time you switched back to the monitor tab.

## Design

**URL-driven tabs** — Tab selection is now managed via `useSearchParams()` instead of local `useState`. `ActiveDetailView` reads the `?tab=` param directly and writes it back on click with `setSearchParams({ replace: true })`. The `monitorLocked` gate is a render-time computation: if the deployment is in a deploying state, the tab falls back to `deployments` regardless of the URL. This eliminates two effects that were fighting over tab state and silently stripping query params.

**Transient navigation state** — The `?from=agents` query param (only used for back button path) is replaced with `location.state`, matching React Router's intended pattern for ephemeral navigation context. `DeployedAgentCard` gains a `linkState` prop forwarded to `<Link>`.

**Dead code removal** — `allowMonitorTab` (never set to true) and `stayOnDeployments` (replaced by URL) are deleted along with the tab-stripping effect.

**Observability cache** — Query keys now use the window name (`1h`, `24h`, `7d`) instead of timestamps, so tab switches and remounts always hit TanStack Query's cache. The `queryFn` still computes fresh time params from `Date.now()` on every fetch. Combined with `staleTime: 0`, this gives instant cached renders with an immediate background refetch using current timestamps. The request volume chart skips its entry animation when rendering from cache so that a background refetch completing mid-animation doesn't cause a visible jump.

## Migration

No changes required. The `?from=agents` param is no longer read from the URL; existing bookmarks or links with it are harmless (ignored). `?tab=monitor` deep links now work for all non-deploying agents.
