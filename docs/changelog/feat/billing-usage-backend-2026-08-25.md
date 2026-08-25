# Billing server: drop the dead balances endpoint, thread the real period start, harden quota-increase auth

## Summary

Backend half of a client-side hardening pass on the Billing and Usage
settings pages (see the paired `feat/billing-and-usage-ui-update` branch,
which targets this one). Four independent changes: remove the
`/billing/balances` endpoint now that nothing on the client calls it,
carry the billing period's real start date through so the client no
longer has to approximate it, add a real daily-rated spend endpoint for
the Usage page's chart, and close a couple of auth/validation gaps in
`RequestQuotaIncrease`.

## Design

**`/billing/balances` and the `Balances` provider method are gone.** The
client's Credits & Commits tab that read this endpoint was removed on the
frontend branch; nothing has called it since. Removed end to end: the
route registration in `main.go`, the `GetBillingBalances` handler, the
`Balances` method on the `BillingProvider` interface, both real
implementations (Metronome, no-op), and the test fake. `docs/01-spec/metronome-billing-implementation.md`'s
endpoint table is updated to match.

**`current_period_start` is threaded through from the same draft invoice
as the end.** `GetBillingSpend` only ever returned `current_period_end`;
a client rendering a daily-usage chart had to approximate the start by
stepping the end back one calendar month, which could disagree by a day
on a short-month anchor. The provider's draft invoice already carries
`start_timestamp` alongside `end_timestamp`, so `Spend.CurrentPeriodStart`
now reads it the same way `CurrentPeriodEnd` already did, and
`BillingSpendResponse` exposes it as `current_period_start` (`omitzero`,
so an older client or a response predating this field sees nothing extra).
Covered by `TestCustomerSpend_ReadsCurrentPeriodFromDraftInvoice`, which
stubs a draft invoice response and asserts both timestamps land.

**A new endpoint gives the Usage page a real daily dollar figure.**
The existing `/billing/usage` reads Metronome's raw usage-list endpoint,
which never rates a quantity metric: a Compute Units row is CU-hours at
any window size, with no dollar figure to sum, so a single daily "Spend"
chart combining Compute and AI Gateway usage couldn't be built from it.
Metronome's invoice has a separate, already-rated breakdown
(`ListBreakdowns`) that slices the invoice itself into day (or hour)
windows, each with real per-line-item dollar totals. `Provider.DailySpend`
calls it and sums each day's `usage`-typed line items (reusing
`usageSpend`'s exclusion of credit-drawdown lines, so a day's figure means
the same thing here as the period total does), returning one
`DailySpendPoint{Day, Amount, ByProduct}` per day. `ByProduct` breaks the
same total down by line item name (`usageSpendByProduct`, one more loop
over what `usageSpend` already walks), so a client that needs the
Compute/Models split reads it straight from the rated breakdown instead of
approximating it against a raw usage quantity. Exposed at
`GET /billing/usage/daily-spend`, sharing the same `from`/`to`
window-parsing as `/billing/usage` (factored into `parseBillingWindow`).
Covered by `TestDailySpend_SumsUsageLineItemsPerDayExcludingCredit` and
`TestDailySpend_BreaksAmountDownByProduct`.

**A day can be covered by more than one invoice, and `parseBillingWindow`
had no bound on how wide a caller could ask for.** Two fixes from the same
review pass:
- A customer can have a finalized invoice for a past period abutting a
  draft one for the current period, or a correction that supersedes an
  earlier invoice. The first version of `DailySpend` appended one point
  per breakdown with no folding, so a voided invoice's breakdown counted
  its line items alongside its replacement's, doubling that day, and two
  non-void invoices covering the same day would have summed instead of
  one superseding the other. `DailySpend` now drops any breakdown whose
  invoice status is `VOID` outright, and folds by day, keeping only the
  most recently issued non-void invoice per day. Covered by
  `TestDailySpend_DropsVoidedInvoices` and
  `TestDailySpend_TwoNonVoidInvoicesForTheSameDayKeepsTheLatestIssued`.
- `parseBillingWindow` accepted any `from`/`to` a caller sent. The
  breakdown endpoint pages 35 days at a time, so a caller asking for a
  multi-year window (by mistake or otherwise) paged through it serially
  until the request's own `WriteTimeout` cancelled the context, and the
  caller saw only a bare 502 with nothing pointing at the window as the
  cause. `parseBillingWindow` now refuses (400, naming the bound) any
  window wider than `maxBillingWindowDays` (366, a full year), before
  either endpoint it feeds reaches the provider. Covered by
  `TestParseBillingWindow_TheBoundItselfIsAllowed` and
  `TestParseBillingWindow_PastTheBoundIsRefusedWithA400`.

Two caveats worth knowing, both inherited from Metronome rather than
introduced here: a breakdown only reflects usage already folded into the
invoice, so a day right up against "now" can lag slightly behind
`/billing/usage`'s raw rows until the invoice catches up; and each page
tops out at 35 daily windows, fine for a calendar month, which is what
`ListBreakdownsAutoPaging` (used here) pages through automatically if a
caller ever asks for more.

**`RequestQuotaIncrease` authenticates properly and trims the reason.**
The handler read the user id with `c.Get("user_id")` and ignored the
type-assertion failure, so a request that reached the handler without a
resolved user silently inserted an empty `requested_by`. It now resolves
the user through `middleware.GetUser` and returns 401 when absent. Reason
validation only rejected an exactly-empty string, not a whitespace-only
one; the handler now trims before checking, matching a client that
already trims and blocks the same input. Both are covered by new tests
(`TestRequestQuotaIncrease_Unauthenticated`,
`TestRequestQuotaIncrease_WhitespaceOnlyReason`), and the existing success
test now pins `requested_by` to the authenticated user's id via
`WithArgs`, so a handler that reads the wrong context key fails loudly
instead of silently.

## Migration

None. All changes are additive (`current_period_start` is a new,
`omitzero` field) or remove a code path with no remaining caller. No
existing request or response shape is broken for a client that hasn't
adopted the new field yet.
