## Summary

After a GitHub OAuth redirect back to the new blueprint wizard, the source step card appeared blank instead of showing the GitHub-connected state with the repo picker.

## Design

The previous fix used `useLayoutEffect` to restore wizard state (active step, name, org, visibility, GitHub connected flag) before the first paint. While layout effects do fire before paint, the browser's CSS transition system still detects the carousel transform change from `translateX(0%)` to `translateX(-25%)` and plays the 0.3s animation. During that animation the source card (step index 1) enters from off-screen, and since `overflow-hidden` clips it until it fully slides in, the user sees a blank white card.

The fix uses lazy `useState` initializers instead. A `readOAuthReturn()` helper reads `window.location.search` and `sessionStorage` synchronously and is passed directly as the initializer to each affected `useState`/`useReducer` call. Because the correct state is set on the very first render, the carousel starts at `translateX(-25%)` with no prior style to animate from — no CSS transition fires and the source step renders fully visible immediately.

Cleanup (sessionStorage removal and URL param stripping) happens in a single `useEffect` as before.

## Migration

No action required.
