## Summary

Two fixes to the blueprints page loading experience: eliminate the empty-state flash on refresh, and replace the centered spinner with shaped skeleton cards backed by an SSR loader.

## Design

**Empty state flash fix**

When auth/account data hasn't resolved yet, `useAccountBlueprints` is disabled so `isLoading` is false with no data — causing the empty state to flash before the query fires. Guard with `!isReady` so the spinner (now skeletons) shows until `activeAccount` is available.

**SSR loader + skeleton cards**

Added a `loader` to `Blueprints.tsx` (same pattern as `AgentDashboard`) that runs server-side, fetches the blueprint count for the user's personal account, and returns it as `loaderData.count`. This count is passed as `skeletonCount` to `BlueprintListView`.

`BlueprintCardSkeleton` mirrors the real card layout — avatar placeholder, title and description bars, footer row — rendered in the same responsive grid as the real cards. The page now lands with its initial render already shaped correctly; no spinner, just the right number of skeleton cards that swap in place when the client query resolves.

The skeleton count is a best-effort hint using the personal account. Org-scoped views may show a slightly different number of skeletons since the active account is stored in `localStorage` and is not readable server-side.

The shared `createServerApi + getCurrentUser + find personal account` boilerplate was extracted into `getPersonalAccount(request)` in `api.server.ts`, used by both `AgentDashboard` and `Blueprints` loaders.

## Migration

No action required.
