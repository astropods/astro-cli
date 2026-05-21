# feat(progress-bar): nprogress-style trickle for navigation feedback

## Summary

The top-of-viewport `NavigationProgressBar` was a perpetual 1.1s sweep — it looked identical at second 1 and second 5 of a slow navigation, with no acknowledgement of when the click was registered or how close the page was to ready. Replaces the sweep with the GitHub / Turbo / nprogress pattern: jump fast, trickle slow, snap to done.

## Design

Only `apps/astro-client/src/components/IndeterminateProgressBar.tsx` changes. `NavigationProgressBar.tsx` stays as-is — it already correctly drives an `active: boolean` off `useNavigation` + `useRevalidator` (and deliberately avoids `useIsFetching`, which would otherwise pin the bar forever while deployment-status polling runs every 3s elsewhere in the app).

The bar is now a small state machine on `active`:

| Event | Behavior |
|---|---|
| `active` flips `true` | Snap to 15% immediately (acknowledges the click) |
| Still pending | Trickle toward 90% (`+= (90 - p) * 0.05` every 200ms — asymptotic, never reaches) |
| `active` flips `false` | Snap to 100%, fade opacity to 0 over 300ms, unmount |
| Rapid `false → true` while finishing | Cancel the hide timer, reset to 15%, restart trickle |

Width is animated via plain CSS `transition: width 200ms ease-out` — no Motion dependency. Opacity has a 300ms ease transition on the wrapper. Both intervals + timeouts are cleared in the effect's cleanup, so unmount mid-trickle is safe.

```ts
const INITIAL_PROGRESS = 15;
const TRICKLE_CEILING = 90;
const TRICKLE_INTERVAL_MS = 200;
const TRICKLE_DECAY = 0.05;
const FINISH_MS = 300;
```

These are inlined as constants at the top of the file so they're easy to tune without digging through the effect.

**Why not track real query speed?** That would require either `useIsFetching` (broken by the polling above) or a custom in-flight-queries-scoped-to-this-navigation tracker — both add complexity without changing the user experience meaningfully. The state machine *suggests* progress and it feels honest at every wait length: a 50ms navigation gets a quick snap-to-100 fade; a 5s navigation gets a slow burn that's still moving. Same UX GitHub uses.

## Migration

None. Single component swap. The lifecycle source (`NavigationProgressBar` → `active` boolean) is unchanged, so every existing call site picks up the new behavior automatically.
