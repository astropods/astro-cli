## Summary

Hard-refreshing `/insights` in production rendered a blank page. Client-side
navigation from the nav bar worked, but a refresh returned a 500 with the SSR
log line `"The render was aborted by the server without a reason"` preceded by
recharts warnings about `width(-1) and height(-1)`.

Root cause: `5aa0072ab` ("rip Insights skeletons", 2026-05-19) removed the
`SkeletonChart` placeholder that used to gate `CostOverTimeChart` while loading.
That placeholder was a plain `<div>` — it incidentally kept recharts off the
SSR path. After the rip, the chart renders `ResponsiveContainer` directly. On
the server there is no DOM, so recharts measures the parent at `width=-1
height=-1`, throws, and trips `onError` in `entry.server.tsx`, aborting the
stream. Client-nav avoided it because by then the `h-[300px]` parent was in
the live DOM and could be measured. `ActiveUsersSpendChart` landed later with
the same shape and inherited the bug.

## Design

Both chart components now defer recharts to after hydration via a local
`useHydrated()` hook (`useState(false)` → `useEffect(() => setHydrated(true))`).
SSR and the first client paint render the same `"Loading chart..."` fallback
inside the existing `h-[300px]` slot, so there is no hydration mismatch. Once
hydrated, the real chart mounts client-side with measured dimensions.

The guard is per-chart, not page-level: stat cards, the top spenders table,
and all page chrome continue to SSR normally. Only the two recharts panels
swap to a placeholder during SSR, which keeps the perceived load cost minimal
on refresh.

Skeletons are deliberately not reintroduced — the failure mode is recharts on
SSR, not the skeleton rip's intent. A per-chart hydration gate fixes the issue
directly without bringing back the loading flash that motivated the rip.

## Migration

No user action is required.
