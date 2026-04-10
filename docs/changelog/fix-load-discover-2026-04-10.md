# Fix direct-load 500 on /blueprints/discover

## Summary

Directly loading `/blueprints/discover` (or any SSR route that renders avatars) threw an "Unexpected Server Error" and fell back to client rendering. Images in blueprint cards and profile avatars also failed to show their fallback states on direct load.

## Design

**`useSyncExternalStore` missing server snapshot.** The `useAvatarBust` hook in `avatar-bust.ts` called `useSyncExternalStore` without a `getServerSnapshot` argument, which React requires for server-rendered components. The fix adds `() => undefined` as the server snapshot — correct because blob URL overrides are browser-only and always absent on the server.

**Pre-hydration `onError` miss.** Images in SSR HTML are fetched by the browser before React hydrates and attaches event handlers. If a CDN image 404s during that window, `onError` never fires and the broken image persists. `BlueprintIdentity` and `UserAvatar` now check `img.complete && img.naturalWidth === 0` in a `useEffect` after mount and trigger their fallbacks if the error was already missed.

## Migration

No action required.
