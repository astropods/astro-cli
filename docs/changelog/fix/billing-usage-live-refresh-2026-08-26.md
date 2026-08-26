# Refresh the usage page on focus, on arrival, and on demand

## Summary

The usage page read its numbers once on mount and then never again. Spend that
landed while someone watched the page did not appear until they navigated away
and back, and even a remount inside the 60 second `staleTime` was served from
cache. Switching tabs did not help either: `refetchOnWindowFocus` is off
globally, so a tab kept whatever it held when it lost focus.

Nothing else in the chain was slow. Measured in preview against a probe agent
making one gateway call every five minutes:

| Hop | Lag |
|---|---|
| Provider answers to Metronome's rated invoice breakdown, which is what the page reads | 3.4 s to 7.4 s |
| `astro-server` | none; `billingData` calls the provider per request with no cache |
| Client | unbounded; no refetch at all |

The first row is bounded by polling the breakdown every four seconds across a
known call. The call completed at 14:37:11.6, the breakdown did not carry it at
14:37:15, and did at 14:37:19, rising by exactly that call's cost. So bifrost's
emit, the collector, Metronome's ingest and Metronome's rating together cost
under eight seconds. The client was the whole delay.

## Design

**Three moments, no timer.** Arriving at the page, returning to the tab, and
asking. `useBillingSpend`, `useBillingUsage` and `useBillingDailySpend` set both
`refetchOnMount` and `refetchOnWindowFocus` to `'always'` for the open period,
and both billing screens carry a Refresh button in the header.

`'always'` rather than `true` on both. `true` only refetches data React Query
already considers stale, so against a 60 second `staleTime` a tab regaining focus
inside that minute would show whatever it held when it lost focus, which is the
one case the focus trigger exists for. Arriving at the page and returning to it
are both requests for the current number, not for whatever was cached.

No interval. Spend reaches the provider in under eight seconds, so any interval
short enough to be worth having would spend three provider calls a tick on a page
nobody is reading.

A closed period keeps the old `staleTime` and neither trigger, because its
figures cannot move. `UsageView` already derived `isCurrentPeriod` for its own
rendering; that derivation moves into the query hook so one answer drives both
what renders and what refreshes.

**One button, both screens.** Spend is shown in two places: the Usage page, and
the pay-as-you-go card on the Billing page. Both now have the same Refresh
control in their section header, from one `RefreshButton` in `SettingsShared`.

It invalidates the `['billing', account]` key prefix rather than calling a list
of `refetch` functions. The figures on a screen are spread across separate
queries owned by separate components: the Usage header's total and the two
streams it splits into, and on the Billing page the spend card and the invoices
table. A button wired to a subset would leave a total disagreeing with the rows
it is made of, and would need rewiring whenever a screen gained a query. The
prefix reaches whichever queries the page has mounted. It disables while any of
them is in flight.

Compute has a floor none of this touches. `MeteringWorker` is a River periodic
job on a five minute interval, and `completedWindows` only emits windows that
have fully closed, so a compute window becomes visible five to ten minutes after
it ends. Gateway spend has no such floor.

## Migration

None. Read path only.
