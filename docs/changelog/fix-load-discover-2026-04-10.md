# Fix direct-load 500 on /blueprints/discover

## Summary

Directly loading `/blueprints/discover` (or any SSR route that renders avatars) threw an "Unexpected Server Error" and fell back to client rendering. Images on SSR-rendered pages also failed to show their fallback states on refresh.

## Design

Two separate issues were fixed.

**`useSyncExternalStore` missing server snapshot.** The `useAvatarBust` hook in `avatar-bust.ts` called `useSyncExternalStore` without a `getServerSnapshot` argument, which React requires for server-rendered components. The fix adds `() => undefined` as the server snapshot — correct because blob URL overrides are browser-only and always absent on the server.

**Pre-hydration `onError` miss.** Images that are part of SSR HTML (e.g. blueprint cards populated by the route loader) are fetched by the browser before React hydrates and attaches event handlers. If a CDN image 404s during that window, `onError` never fires and the broken image or broken icon persists. Both `BlueprintIdentity` and `UserAvatar` now add a `useEffect` that runs after mount and checks `img.complete && img.naturalWidth === 0` — the browser's signal for a failed load — and triggers the fallback if the error was already missed.

Images rendered after client-side hydration (e.g. the nav avatar, which depends on the auth fetch) are unaffected and did not need changes.

## Migration

No action required.
