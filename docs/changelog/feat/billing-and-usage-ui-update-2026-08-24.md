# Harden Billing and Usage against load failures, permission gaps, and a few state bugs

## Summary

The Billing and Usage pages from `billing-usage-split-2026-08-21.md` treated
a failed query the same as `data.available: false`, gated billing controls
on a role that's never set outside an org context, and had a handful of
smaller state bugs (a shared mutation's pending flag disabling every row in
a list, a local-time date next to otherwise UTC-pinned ones, a swallowed
save-side failure signal). This closes all of those, found across several
review passes over the pages' own components.

This is the client half of the work: it targets `feat/billing-usage-backend`,
which carries the server-side changes this branch depends on (dropping the
dead `/billing/balances` endpoint and threading `current_period_start`
through). See that branch's own changelog for the backend design writeup.

## Design

**Load failures get their own state.** `SettingsShared.tsx` gains
`LoadError({ message, onRetry })`, built on the existing `ActionPanel`
(matching astro-client/CLAUDE.md's own component-reuse guidance) rather than
a bespoke banner. Every query-backed section on both pages checks
`isLoadingError` ahead of `data.available` and wires the query's own
`refetch` into it: `PayAsYouGoCard`, `PaymentMethod`, `BillingView`'s
`Invoices`, both halves of `ResourceLimitsSection`, and `UsageView`'s spend
and daily-usage queries independently (a failed usage-rows fetch shows its
own retry state under a header that still rendered from a spend query that
succeeded).

The check is `isLoadingError`, not the broader `isError`: React Query sets
`isError` on any failed fetch, including a background refetch of a query
that already holds good data, and leaves that data in `data` either way.
`isLoadingError` is the narrower "this query has never returned data" case,
the only one that should tear down a card that's already showing real
numbers. `PayAsYouGoCard`'s `hasCard` derivation follows the same rule: it
reads `useBillingStatus`'s `data` directly and assumes a card exists only
when that query has never returned anything, so a transient blip after a
successful load can't flip it to "no card."

**Permission gating is scoped to the account being viewed, not the session.**
`canManageBilling(role)` reads the session's currently active *org* role,
which is only ever set by switching into an org — on a personal account's
own billing page, `role` is null for every user, including that account's
sole owner. `canManageAccountBilling(account, personalAccountName, role)`
(`billing-copy.ts`) is true when `account` is the caller's own personal
account, and falls back to the org-role check otherwise. `PayAsYouGoCard`'s
"Increase limit to resume now" and `ManageLimitsDialog`'s inputs both use it.

**A few isolated state bugs, each scoped to one component:**
- `ManageLimitsDialog.onSave` discarded the save mutation's result and always
  toasted "Limits saved," even when the server reports `limit_lift_failed`
  (the write landed but the provider couldn't lift a live suspension, and
  nothing retries that). It now reads the field and warns instead.
- `BillingView`'s `Invoices` instantiated `useDownloadInvoicePdf` once for the
  whole table and disabled every row's Download button off that one shared
  `isPending`. It now tracks the downloading invoice's id locally and
  disables only that row.
- `ResourceLimitsSection`'s `QuotaRequestsTable` formatted `created_at` with
  bare `toLocaleDateString()` (the host's local zone) instead of
  `formatShortDate`, the UTC-pinned helper every other date on these pages
  uses.
- `PaymentMethod`'s inline `"Loading…"` string is replaced with a
  `PaymentMethodSkeleton` shaped like the populated card, since the card's
  own presence decides its header buttons ("Add credit card" vs.
  "Update"/"Remove") and could otherwise render the wrong one for a moment.
- `ResourceLimitsSection`'s two empty states now use the shared `EmptyState`
  component instead of bare paragraphs.
- `AddCardDialog` now starts its SetupIntent through a proper mutation hook
  (`useCreateSetupIntent`) instead of calling `api.createSetupIntent`
  directly, and its Retry button shares the same attempt-counter guard as
  the mount effect, so a slow response from either path can't overwrite a
  newer one.
- `RequestIncreaseDialog` now runs its mutation error through
  `getApiErrorMessage(err, fallback)`, matching every other mutation on
  these two pages, instead of printing the raw error message.

**Shared test fixture.** `api/queries/billing.fixtures.ts` adds
`buildBillingSpend`/`buildSpendResponse` with neutral defaults; the four
test files that each rebuilt the same `BillingSpend` shape now call it,
keeping only the default values specific to what each file tests.

`ManageLimitsDialog` no longer caps the spend limit at $1,000
(`billing-usage-split-2026-08-21.md` introduced that guardrail). It applied
identically to org and personal accounts with no way to raise it in the UI,
so an org spending past $1,000/period could not set its own limit above what
it had already spent. The server already accepted any non-negative amount;
the client-side cap is removed pending a product decision on whether a real
ceiling belongs here. The Save button also no longer disables on a blocking
validation error or an unchanged form — a disabled button drops out of tab
order, and `onSave` already returns early on `blockingError`, so nothing
depended on the button itself being inert.

**Compute and AI Gateway caps are settable again.** An earlier pass on this
branch removed the client-side write path for per-metric usage thresholds
while treating it as dead code; the server's
`PUT /accounts/:account/billing/usage/thresholds` endpoint was never removed
and `BillingSpend.usage` was already flowing to the client unused.
`ManageLimitsDialog` restores Compute and AI Gateway sections alongside the
existing spend row, each with its own alert threshold and limit, seeded from
`spend.usage` and saved through `useSetBillingUsageThresholds`. Saving
several changed rows at once runs them through the same `Promise.allSettled`
pattern as before, naming whichever row failed. `PayAsYouGoCard` follows:
its "Agents paused" badge and message previously only checked the
account-wide spend limit, so a Compute or AI Gateway limit pausing the
account on its own left the card looking fine. It now checks all three and
names whichever are in alarm.

**Two edge cases in that restoration, closed:**
- The paused message named the resume date unconditionally
  (`formatShortDate(periodEnd)`); with `current_period_end` omitted
  (`omitzero` server-side), it rendered a dangling "resume on .". It now
  falls back to "resume when the billing period resets", matching the
  guard `PayAsYouGoCard` already used for the upcoming-invoice line.
- `useRowInputs` re-seeded a row's fields whenever the spend query's
  `isLoading` flipped to `false`, with no regard for whether the account had
  already typed into that row. Opening the dialog against a cold cache and
  typing before the first load landed meant the load's own values could
  overwrite the typed ones. Each row now tracks whether it's been touched and
  skips the seed once it has, resetting on close so the next open seeds fresh.
- `RequestIncreaseDialog`'s successful-submit path called `onOpenChange(false)`
  directly and reset `reason`/`amount` itself, instead of routing through the
  same `handleOpenChange` the manual-close path uses. It never reset
  `reasonTouched` or the mutation, so a blank-reason submit followed by a
  successful one left the "touched" flag set; since the dialog stays mounted
  across opens in `ResourceLimitsSection`, the next open showed "A reason is
  required." against an empty field the user hadn't touched yet. Both paths
  now go through `handleOpenChange`.
- `AddCardDialog` guarded `stripePromise` against a stale SetupIntent response
  overwriting a newer one, but read `clientSecret` straight off the mutation's
  own `data`, which react-query updates to whichever attempt's response
  settled last regardless of the guard. An older response landing after a
  newer one (rapid open + Retry) could pin `Elements` to a stripePromise from
  one attempt and a clientSecret from another. Both now come from local state
  set together inside the same guarded callback.
- `PayAsYouGoCard`'s `hasCard` defaulted to true whenever `useBillingStatus`
  had no data, which correctly avoids flashing the paused banner during a
  transient load or refetch blip, but also fail-opened silently on a
  permanent failure: an account with no card and a status query stuck erroring
  never saw the "add a payment method" warning it needed. A confirmed
  `isLoadingError` now shows its own retry notice instead of assuming either
  way.
- `UsageView`'s daily-usage window reconstructed the period's start by
  stepping the reported `current_period_end` back one calendar month, since
  `BillingSpend` didn't carry the real start. On a day-29-31 anchor whose
  prior month is shorter, that approximation could disagree by a day with
  the server-computed header total sharing the same screen. The client's
  `BillingSpend` type now carries `current_period_start` (added server-side
  on the paired `feat/billing-usage-backend` branch, which this one targets);
  `UsageView` uses it directly when present and falls back to the month-step
  approximation against a server that hasn't started sending it yet.

**Save is disabled until something changes.** `ManageLimitsDialog`'s Save
button previously stayed enabled with nothing to write, so a click with no
edits was a silent no-op: no toast, dialog stays open. It now disables
whenever every row matches its saved value, and re-enables the moment any
row changes (including back to the same value it started with). This
doesn't reintroduce the Rule #2 problem the button's always-enabled state
solved: that rule is about not hiding a validation error behind a disabled
button, and disabling on "nothing to save" hides no error, since
`blockingError` is already rendered inline regardless of the button's state.

**Comment pass.** Went through every comment this branch added or changed
and cut the ones that restated the adjacent code or duplicated another
comment nearby (`PayAsYouGoCard`'s `hasCard` derivation had its rationale
written twice, once as a code comment and once again as a JSX comment a few
lines down). The comments that record a real constraint, trade-off, or
failure mode stay, just shorter.

**Design pass on the upcoming-invoice line.** `PayAsYouGoCard`'s "Upcoming
invoice" row now bolds the label and the amount, and moves the period-end
date to follow the amount instead of the label ("Upcoming invoice ⟷ $45.40
on Aug 23, 2026" instead of "Upcoming invoice on Aug 23 ⟷ $45.40"). The date
itself stays regular weight.

**Per-metric (Compute, AI Gateway) caps are removed from `ManageLimitsDialog`
again, this time intentionally.** An earlier pass on this branch restored
them after finding they'd been accidentally dropped while the backend still
fully supported them. Design's final layout for this dialog only shows the
account-wide spend limit: one cap is a simpler mental model than three
independent warn/stop pairs, and product does not want the account owner to
know Compute and AI Gateway thresholds exist as a concept. This is a
deliberate simplification, not a regression, and it does not touch the
server: `SetBillingUsageThresholds`, the `UsageMetric` enum, and the
`/billing/usage/thresholds` route are untouched, so an account that already
has a per-metric threshold set (from before this change, or from outside
this UI) keeps it, and it keeps firing.

Removed with it, as dead code once nothing in the client calls it:
`useSetBillingUsageThresholds` and `seedUsageThreshold`
(`api/queries/billing.ts`), and `UsageThresholdsInput` and
`api.setBillingUsageThresholds` (`lib/api.ts`). `BillingSpend.usage` and the
read-side `UsageThresholds` type stay: `PayAsYouGoCard` still reads
`spend.usage` to detect a per-metric pause the bar wouldn't otherwise show.

**The pause message stays honest without naming the hidden concept.**
Before this change, a limit-reached message on `PayAsYouGoCard` named every
limit that fired ("Spend limit and AI Gateway limit reached..."). Naming
"AI Gateway" here would expose exactly the concept the dialog no longer
lets the account see or set. The message now reads "Spend limit reached"
when the account's own visible control is what fired (the common case, and
the one whose name matches the dialog's own field), and falls back to the
generic "Usage limit reached" when it's a per-metric cap doing the pausing
instead. Either way, the badge, the destructive bar tone, and the resume
link still work: removing the setting UI does not mean an account already
paused by one of these has no way to know agents stopped or how to unstick
them, it only means the specific cause stays unnamed.

`usage-limits.mdx` is rewritten to describe the single spend cap only: the
Compute and AI Gateway rows, and the note about zero-rated plans capping
usage instead of spend, are removed along with the UI they described.

**A few tests pinned styling instead of behavior.** `PayAsYouGoCard`'s
invoice-line test asserted `font-medium` on specific spans; a designer
changing the weight would fail a test that has nothing to do with whether
the feature works. It's replaced with a test of the actual requirement:
the date renders after the amount, not the label. `ResourceLimitsSection`'s
two empty-state tests asserted `border`/`border-border` on the empty
state's wrapper as a proxy for "uses the shared `EmptyState` component;"
the message text already proves the right content renders, so the class
assertion is dropped.

**The resume link no longer dead-ends on a per-metric pause.** `ManageLimitsDialog`
only edits the account-wide spend limit, but the resume link on `PayAsYouGoCard`
used to show unconditionally whenever any limit was in alarm. Raising the
spend limit through the dialog cannot lift a suspension a hidden Compute or
AI Gateway cap caused, so following that link did nothing, the write itself
still succeeded, and the account stayed paused with no error to explain why.
The link now only renders when the account's own spend limit is what's in
alarm; a per-metric-only pause keeps the "reached and agents will resume on
[date]" copy with no link, since there's nothing the dialog can do about it.

**Per-metric pause detection no longer names the two metrics that happen to
exist today.** `PayAsYouGoCard` checked `spend.usage?.compute` and
`spend.usage?.gateway` by name to catch a pause the spend bar wouldn't show.
`spend.usage` is a `Record<string, UsageThresholds>`, so a limit set on any
future metric would have paused the account server-side while this card kept
showing a healthy state. It now iterates every key in `spend.usage` instead
of naming two, so a metric that doesn't exist yet is covered without a
client change.

**A new Usage settings page ships with public docs.** `usage.mdx` describes
what the page shows (spend split into Compute and Models, a daily chart per
stream, resource limits against their quota, requesting an increase) and who
can see it (every personal account's owner; org admins and owners only).
Linked from `usage-limits.mdx` and `ai-gateway.mdx`, and added to the
Platform section of the docs nav next to Usage limits.

**`DailyUsageCharts`'s `useMemo` now actually memoizes.** `billingPeriod`
constructs a fresh `period` object on every render, so depending on `period`
by identity invalidated the memo every render regardless of whether the
window had changed. It now depends on `period.from`/`period.to`, the
primitive bounds that actually decide the chart's output.

**The daily-usage chart no longer draws empty future days.** `period.to` is
the billing period's scheduled end (Metronome's draft invoice), which for
an open period is still in the future. `periodDayKeys` walked all the way to
it, so the chart zero-filled every day between today and the period's real
end and labeled the last bar with a date that hasn't happened yet. It now
clamps the walked window to the start of tomorrow (UTC), so the chart ends
at today; the header and limit math still read the real period. Exported for
a direct unit test, since Recharts' rendered axis ticks aren't reliably
queryable in this test setup.

**The card no longer says "No spend limit set" next to "reached and agents
will resume."** Both statements were individually true when a per-metric
cap (not shown in the UI) paused an account with no account-wide spend
limit set, but side by side they read as contradictory. The hint is now
suppressed whenever a limit is reached; the pause message is the one signal.

**The daily chart is one "Spend" bar, not one bar chart per metric.**
`DailyUsageCharts` drew a separate chart for Compute (CU-hours, an amount no
dollar figure attaches to) and one for Models (dollars), which read as two
different kinds of measurement stacked on one page and forced a reader to
mentally add them to answer "how much did this account spend today." Both
are replaced with a single `DailySpendChart` in dollars, fed by a new
`useBillingDailySpend` query against
`GET /billing/usage/daily-spend` (server-side design on
`feat/billing-usage-backend`, which this branch targets). The Models
breakdown table is unchanged, still fed by `useBillingUsage`'s raw rows for
its per-model grouping.

`buildDailyChart` (per metric, zero-filled per day) is replaced with
`buildSpendChart` (one series, same zero-fill and period-clamped day keys via
the existing `periodDayKeys`). `PeriodSpend`'s availability check now covers
both `useBillingUsage` (still needed for the breakdown table) and
`useBillingDailySpend`: either query's load failure shows the chart's retry
state, and retrying re-fires both, since a stale header split next to a
retried chart would show two different pictures of the same period.

**`UsageHeader`'s Compute/Models split now reads the daily breakdown's own
rated figures instead of deriving Compute by subtraction.** `splitSpend`
used to sum Models dollars from `useBillingUsage`'s raw rows and get Compute
by subtracting that from the period total, on the assumption that nothing
else shares the total. `feat/billing-usage-backend`'s `DailySpendPoint` now
carries `by_product`, a real rated dollar figure per line item name, for the
same reason the chart itself moved off the raw usage list: Compute Units has
no dollar figure in a raw usage row at any window size, only in a rated
breakdown. `splitSpend` now sums each stream's own `by_product` entry across
`dailySpend` directly, so both figures come from the provider rather than
one of them being inferred. Without a loaded breakdown (no period yet, or a
period with none), both stream totals read as $0 rather than a guess; the
period total above them still reads from `useBillingSpend` regardless.

**`ManageLimitsDialog` no longer tells the account a new limit waits for the
next period when the provider enforces it immediately.** Saving a spend
limit calls `SetCustomerSpendThreshold`, which archives the old alert,
creates the replacement, and resets it, forcing an immediate
re-evaluation rather than waiting for Metronome's next scheduled check.
`selfLimitReached` then compares that limit against this period's own
usage spend on the very next status read. A limit saved below what the
account has already spent this period pauses its agents right away, not
at the next period rollover the dialog used to name. The warning shown
next to the spend-limit field now says saving pauses agents immediately
instead of naming a start date that was never real.

**`AddCardDialog`'s stale-response guard now actually covers closing the
dialog.** Its `attempt` ref was meant to guard a slow SetupIntent response
from landing after a newer attempt or after the dialog closed, but closing
only reset local state; it never advanced `attempt.current`. A request
still in flight when the dialog closed could answer later with
`mine === attempt.current` still true, writing its `clientSecret` and
`stripePromise` into state anyway. Reopening the dialog would then mount
`Elements` against that abandoned secret for a moment before the fresh
attempt's own response replaced it. Closing now bumps `attempt.current`
alongside the reset, so an abandoned response fails its own check instead
of writing in.

## Migration

None. All changes are additive or internal to existing components; no
props, routes, or query hooks changed shape.
