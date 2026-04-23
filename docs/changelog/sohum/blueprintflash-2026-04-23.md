## Summary

On refresh, the Blueprints page briefly showed the empty state before the blueprint list appeared. This happened because when auth and account data haven't resolved yet, the query is disabled — meaning `isLoading` is `false` even though no data exists — causing the empty-state branch to render immediately.

## Design

`isReady` (a local flag for `isAuthenticated && !!activeAccount`) is now passed as `!isReady || isLoading` to `BlueprintListView`. While auth/account data are still resolving the spinner renders instead of the empty state. Once `activeAccount` is available the query fires normally.

## Migration

No action required.
