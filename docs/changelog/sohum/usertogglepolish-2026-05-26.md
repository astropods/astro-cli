# Insights: client-side range slicing + collateral cleanup

## Summary

Date-range toggles on the Insights page were laggy: the URL flipped
instantly but each chip change fired a fresh server query, and TanStack's
`keepPreviousData` left stale numbers under the new range label until the
request returned. Refreshing felt like the fix because the loader primed
the cache before render.

The page now fetches all-time data **once per session** and derives every
view from it client-side. Toggles become 0ms in-memory recomputations.
The architecture shift also let us delete a chunk of now-dead server code
and fixed a handful of long-standing chart / empty-state / labelling
bugs in the same pass.

## Design

### Range scoping moves to the client

The Insights loader always fetches all-time. Both hooks read the URL's
range chip and produce sliced views via pure-JS memos:

- **Agents view** (`useInsightsData`) — `sliceBlueprintsResponseByRange`
  walks each blueprint's `cost_over_time` / `requests_over_time` /
  `tokens_over_time`, trims to the URL window, and recomputes `cost_usd`,
  `requests`, `total_tokens`, `cost_per_request`, `tok_per_request`.
  Headline cards, chart, and Top Spenders all read from the sliced
  response.
- **Users view** (`useUsersInsightsData`) — `sliceUsersByRange` walks the
  per-(day, user) data **once** and returns both shapes the view needs:
  per-user totals (with a window-scoped `last_seen`) for Top Spenders +
  filter chips, and per-day sparkline arrays (optionally filtered by the
  selected-user set) for StatCards.

`shouldRevalidate` only re-runs on org-switch and view toggle
(agents ↔ users). Range chips and filter chips never hit the network.

Two small additive server changes support the users-view path:
`GetAccountLangfuseSummary` with `group_by=user` now pulls cost + count +
totalTokens per (day, user), and `AccountUserCost` gains `Requests` +
`Tokens` fields, so the client has everything it needs from one all-time
fetch.

### What falls out of the slicing move

**Change %: same semantic, different machinery.** Change % used to be
computed server-side via a second `accountDailyMetrics` call against a
prior-period window. With client-side slicing we want to keep the
"this week vs last week" semantic but avoid an extra round-trip per
toggle. The client now derives prior-period totals from the all-time
data it already holds: `shiftPriorWindow(fromDate, toDate)` returns the
same-length window immediately before the current one, then
`sumBlueprintsWindow` / `sumUsersWindow` produce `{cost, requests,
tokens}` totals over that window. The hook passes the prior totals into
`buildFilteredSummary` / `recomputeTotalsFromUsers`, both of which call
a unified `computeChange(current, prior)` helper.

The server-side prior-period code (`shiftPrior`, `pctChange`, the
`if hasPeriod { … }` errgroup branch, the `Change` block in
`buildAccountSummary`) stays in place — the Insights client just doesn't
opt in (omits `from`/`to`), so the prior-period query never fires for
it. Future or external callers that send `from`/`to` still get
server-side change% as before.

**Chart axis bug fixed.** `/insights?range=30d` used to show the first
bar at whichever day activity began, not at the start of the requested
window — the data-shaping helpers took the union of dates that had data,
and `CostOverTimeChart.dayKeysForRange` drifted from the server's
`period.end` via `new Date()`. `enumerateDates(periodStart, periodEnd)`
now walks the bounded UTC window and is the canonical axis at all three
sites (`buildFilteredSummary`, `agentCostOverTime`,
`buildUserCostOverTime`). All-time still falls back to the union of
active dates.

**Empty-state parity.** `useUsersInsightsData.hasData` now reads off the
SLICED users, so an empty range falls through to the page-level
`EmptyState` matching the agents tab (was previously falling through to
per-component placeholders).

### UX side-effect

The architecture work above also brings the change-% delta badge to the
users-view StatCards. Before this PR the badge only rendered on the
agents view (server-side prior-period); now both views derive it
client-side via `recomputeTotalsFromUsers` + the prior-period totals,
so the two surfaces stay consistent.

UI polish that's purely cosmetic — Top Spenders linkify, the
Unauthorized → Unidentified rename, and the 5-min background refetch —
moved to a separate PR (`sohum/insightuipolish`) so this one stays
focused on the architecture.

### Tests

New deterministic suites (`vi.useFakeTimers` to pin "today") covering
`sliceUsersByRange` (both outputs), `recomputeTotalsFromUsers`,
`sliceBlueprintByWindow`, and `sliceBlueprintsResponseByRange`.

## Trade-offs

- **Initial payload** — all-time blueprints + summary fetched once per
  session, ~1–5 MB JSON for typical orgs (mostly arrays of dates +
  numbers, gzip-friendly). Acceptable in exchange for instant toggles.
- **`top_model` + `p95_latency_ms`** — agents-view stays at all-time,
  not range-scoped (no per-day data on the blueprint shape).
- **`agents_used`** — users-view chip stack is all-time for the same
  reason.

## Migration

None. Server changes are strictly additive; the client now ignores
`from`/`to` for the Insights page but external consumers of the
endpoints keep working unchanged.
