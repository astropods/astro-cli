# Fix direct-load 500 on /blueprints/discover

## Summary

Directly loading `/blueprints/discover` (or any SSR route that renders avatars) threw an "Unexpected Server Error" and fell back to client rendering. Images on SSR-rendered pages also failed to show their fallback states on refresh, and flashed between states when switching blueprint tabs.

## Design

Three separate issues were fixed.

**`useSyncExternalStore` missing server snapshot.** The `useAvatarBust` hook in `avatar-bust.ts` called `useSyncExternalStore` without a `getServerSnapshot` argument, which React requires for server-rendered components. The fix adds `() => undefined` as the server snapshot — correct because blob URL overrides are browser-only and always absent on the server.

**Pre-hydration `onError` miss in `UserAvatar`.** Images that are part of SSR HTML are fetched by the browser before React hydrates and attaches event handlers. If a CDN avatar 404s during that window, `onError` never fires and the broken icon persists. `UserAvatar` now adds a `useEffect` that checks `img.complete && img.naturalWidth === 0` after mount and manually triggers the fallback URL swap if the error was already missed.

**Flash-free loading in `BlueprintIdentity`.** Rather than toggling between an `<img>` and an SVG fallback (which causes a visible flash of the broken-image state), `BlueprintIdentity` now always renders the generated SVG identity as a base layer. The CDN image is overlaid at `opacity-0` and transitions to `opacity-100` only via `onLoad`. Failed images stay invisible and the SVG shows through — no broken icon, no flash. A module-level cache of successfully-loaded URLs lets remounted components (e.g. on tab switch) skip the `opacity-0` state entirely.

Images rendered after client-side hydration (e.g. the nav avatar, which depends on the auth fetch) are unaffected and did not need changes.

## Migration

No action required.
