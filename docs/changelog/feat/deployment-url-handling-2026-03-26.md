# URL-driven tabs, observability cache, and deploy reveal state

## Summary

The deployment detail page had broken tab navigation (no back button support, deep links not working, effects fighting over state), observability data refetched from scratch every time you switched back to the monitor tab, and the post-deploy reveal overlay used visible URL params that leaked deployment IDs into the address bar.

## Design

**URL-driven tabs** — Tab selection is now managed via `useSearchParams()` instead of local `useState`. `ActiveDetailView` reads the `?tab=` param directly and writes it back on click with `setSearchParams({ replace: true })`. The `monitorLocked` gate is a render-time computation: if the deployment is in a deploying state, the tab falls back to `deployments` regardless of the URL. This eliminates two effects that were fighting over tab state and silently stripping query params.

**Transient navigation state** — The `?from=agents` query param (only used for back button path) is replaced with `location.state`, matching React Router's intended pattern for ephemeral navigation context. `DeployedAgentCard` gains a `linkState` prop forwarded to `<Link>`.

**Deploy reveal via router state** — The post-deploy reveal overlay (card animation on the My Agents page) previously used visible URL search params (`revealDeploymentId`, `revealAgentName`) and localStorage-based "seen" tracking. Both are replaced with `location.state`. Router state is invisible in the URL bar, not bookmarkable or shareable, and ephemeral by nature — eliminating the need for localStorage guards. `showReveal` is initialized directly from state on mount rather than via `useEffect`, avoiding a dismiss flash caused by a race between `setShowReveal(false)` and the async `navigate` that clears state. On dismiss, the current history entry is replaced with empty state. On "View deployment", `history.replaceState` cleans the `/agents` entry in-place before pushing the deployment detail route, so the back button lands on a clean `/agents` page instead of skipping back to the deploy form.

**Dead code removal** — `allowMonitorTab` (never set to true), `stayOnDeployments` (replaced by URL), and the `first-deploy` reveal flow on `DeployedAgentDetail` (never triggered — nothing in the codebase set `?reveal=first-deploy`) are deleted along with all associated localStorage logic and the `LiveRevealOverlay` render path from the detail page.

**Observability cache** — Query keys now use the window name (`1h`, `24h`, `7d`) instead of timestamps, so tab switches and remounts always hit TanStack Query's cache. The `queryFn` still computes fresh time params from `Date.now()` on every fetch. Combined with `staleTime: 0`, this gives instant cached renders with an immediate background refetch using current timestamps. The request volume chart skips its entry animation when rendering from cache so that a background refetch completing mid-animation doesn't cause a visible jump.

## Migration

No changes required. The `?from=agents` param and `revealDeploymentId`/`revealAgentName` URL params are no longer read; existing bookmarks or links with them are harmless (ignored). The `astro:agents-reveal-seen:*` and `astro:deploy-live-reveal:*` localStorage keys are no longer written or read and can be ignored. `?tab=monitor` deep links now work for all non-deploying agents.
