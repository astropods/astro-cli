# Show loading bar on org switch after caching

## Summary

Switching orgs on a warm, heavy page (e.g. `/agents` with dozens of cached deployment cards) made the top progress bar unreliable: it often appeared late or not at all, and the org switcher dropdown could pop back open mid-switch. The bar was already wired to React Router revalidation, but two things broke the UX on cached paths — paint was blocked by the heavy outlet re-render happening in the same turn as the switch, and the bar cleared as soon as the revalidator went idle (~40ms) even though the page was still swapping content. The dropdown lingered for the same reason: Radix could not finish closing its portal while the main thread was busy unmounting cards.

## Design

Org switching keeps `revalidator.revalidate()` as the loader signal. A small external store (`org-switch-progress.ts`) holds a synchronous “org switch in flight” flag that only `NavigationProgressBar` and `OrgSwitcher` subscribe to, so flipping it does not re-render the heavy outlet.

On a real org change, `setActiveAccount` writes the cookie, sets the external flag immediately, then defers work across two animation frames: revalidate first, then apply the account override inside `startTransition`. That lets the progress bar paint before the expensive agents grid re-render. The flag clears only after revalidation is idle, the override transition has finished, and any account-scoped TanStack fetches have settled (with a double-rAF fallback on warm cache when no fetch occurs).

`NavigationProgressBar` treats the external flag like navigation/revalidation activity — still not `useIsFetching`, for the same polling/stale-placeholder reasons as before. `IndeterminateProgressBar` uses a layout effect so the bar becomes visible on the same frame `active` flips, and keeps a minimum width while active so a fast revalidator finish does not collapse it before paint.

`OrgSwitcher` also subscribes to the org-switch flag: it forces the menu closed, blocks reopen until the switch completes, blurs the trigger, and unmounts `DropdownMenuContent` while switching so the portal cannot linger visibly during a heavy render.

## Migration

None.
