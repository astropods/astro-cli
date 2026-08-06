# Insights: report the window the data actually covers

## Summary

The Insights page anchored every window on the wall clock, on both sides and computed independently — the server built `[today-(days-1), today]`, and the client zero-filled its chart axis from `new Date()`. On the rollup-backed read path that trailing day can never hold data: the fact table stores complete days only, so `DaysToRoll` stops at yesterday and today is left for a live overlay that isn't built yet.

Two things followed from that:

- **A permanently empty trailing bucket.** The last bar of every chart was zero all day, every day, which reads as lost data rather than as a day that hasn't finished.
- **Systematically negative deltas.** `priorWindow` shifts back by the current window's own span, so a "7d" card compared six days of spend against seven complete prior days. Every stat-card change was biased down by roughly a day's worth of usage.

The page now reports the window it actually covers instead of the one the clock implies.

## Design

**The window ends on a data horizon, not on `now`.** `insightsPeriod`'s anchor is the last day the window covers — the caller's horizon. The Langfuse path keeps passing today, which it does have data for. The rollup path passes its watermark, so charts, stat cards, the range-scoped tables, and the prior-window comparison all shift together and the delta becomes like-for-like by construction. `priorWindow` needed no arithmetic change: its input was what was wrong.

The horizon is the `(account, agents)` watermark, clamped to the last complete UTC day and falling back to it when no watermark exists. A watermark held back by a failed roll-up is therefore *visible* rather than silently papered over with an empty day — the window shortens and the page says which day it covers.

**`as_of` joins the response contract.** `InsightsResponse.as_of` (YYYY-MM-DD, UTC) names the day the data is complete through. It is omitted on the Langfuse path, which has no watermark, and on a cold account whose roll-up has never run — absent means "windows end today", which is what the client already assumes.

**The client renders the window it was given rather than recomputing it.** The chart axes take the reported `period.end`; the header date label comes from the response, keyed to its range so a range switch falls back to the local estimate for one frame instead of mislabelling; the footnote names the coverage day when the server reported one. Client-side date arithmetic remains only as the pre-first-response fallback.

This makes the today-overlay described in `docs/01-spec/insights-rollup-spec.md` purely additive when it lands: it extends the horizon forward by a day rather than changing the shape of the window.

## Migration

None. `as_of` is additive and optional, and the Langfuse read path is unchanged.
