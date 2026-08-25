# Split Billing and Usage into separate pages

## Summary

The Credits tab from `billing-credits-tab-2026-08-20.md` shipped as one of four
tabs on a single Billing page (Usage, Invoices, Credits, Quotas). Updated
designs call for two separate top-level settings pages instead: **Billing**
(can agents keep running: pay-as-you-go standing, payment method, invoices)
and **Usage** (where did the spend go: daily usage, per-model breakdown,
resource limits). This rewrites the tab into that shape rather than
adding a fifth tab.

## Design

`/settings/usage` and `/settings/org/:orgSlug/usage` are new routes, gated the
same way Billing is for orgs (admin/owner only). `SettingsLayout` and
`OrgSettingsLayout` grew a Usage nav item next to Billing.

**Billing** (`BillingView.tsx`) is now three sections, no tabs: a
`PayAsYouGoCard`, `PaymentMethod` (restyled as a "Payment details" card:
card on file, billing cycle, billing email), and the invoices table.

`PayAsYouGoCard` replaces `CreditBalanceCard`. It answers what the account can
spend right now with one figure and one badge rather than five derived modes:
usage this period, a badge for free credit (ready / left / applied) or
"Agents paused", a progress bar against the account's own spend limit, and the
upcoming invoice. The three money figures are stacked so they add up on
screen: usage this period, credit applied as a negative, then the invoice that
follows from them. A bar renders only against a limit the account actually
set, since a bar whose max is its own value reads as "at your cap" when it
means the opposite. The card takes callbacks for its two cross-section
actions rather than scrolling to element ids that only exist on the Billing
page, and it offers the invoices link only when `has_last_invoice` says there
is a finalized invoice to open. The credit-applied figure is
`usage_spend - current_spend`, both already rated by Metronome, so it is not a
re-derivation but the one number the provider already implies.

A banner above the card covers the no-payment-method case that used to be a
fifth mode. It stays up even when the spend limit is what stopped the agents,
because both facts are true at once and only one of them is actionable: in
that state the "Increase limit to resume now" link is disabled behind a
tooltip, since raising a limit resumes nothing without a card to bill.

Invoice rows without a PDF show a disabled download rather than a dash. Only
finalized invoices have one, so the tooltip names the reason: a draft says
when the period closes, anything else says there will be no PDF.

`ManageLimitsDialog` replaces `SpendControls`'s per-metric card. The account
sets one alert and one spend limit, not one per metric (compute and AI Gateway
limits are gone from the UI; the write endpoint and the `BillingSpend.usage`
field are untouched server-side, just unused here now). The dialog validates
inline: a limit above a $1,000 client-side guardrail blocks Save, an alert at
or above the limit blocks Save, and a limit already below this period's spend
warns without blocking (it applies next period, which Metronome's own
reset-on-write behavior already handles). The form seeds from the spend query
once that query settles, not merely when the dialog opens, so opening against
a cold cache cannot save an empty field over a limit the account still wants.

Threshold amounts (`SpendThreshold.amount`) are the credit type's own unit,
Metronome's USD-cents pricing unit, since `SetCustomerSpendThreshold` creates
the alert with `CreditTypeID: usdCentsCreditTypeID`. Every other figure on
`BillingSpend` already arrives in whole dollars. `thresholdDollars()` in
`lib/billing-balances.ts` converts once, so the card and the dialog agree on
scale. (`CreditBalanceCard`'s old display treated the same field as already
dollars, reading the wrong number by 100x for any non-zero threshold. Never
shipped anywhere the discrepancy was visible, since the card and the
per-metric form never rendered on the same page before now.)

**Usage** (`UsageView.tsx`) is the daily usage charts and the Quotas tab's
resource meters, plus a new header and a new spend-breakdown section:

- The header shows total spend against the account's own limit, split into
  Compute and Models, reading usage over the billing period rather than the
  calendar month the endpoint defaults to. Subtracting a month-to-date model
  total from a period spend would report compute the account never spent.
  Models is a direct read of the "LLM Usage" billable metric (already
  `sum(cost_usd)`, so dollars); Compute is the total minus that, which is
  exact because both numbers are already rated. It is not a rate-card guess
  client-side, which `billing-architecture.md` rules out ("Astro never
  computes a price").
- The period is reconstructed as one month back from `current_period_end`,
  because Metronome bills on a fixed day of the month and `BillingSpend` does
  not carry the start date. A fixed 30-day step drops the cycle's first day
  whenever the month is longer, which puts the rows underneath the header out
  of step with the total above it.
- The Models table reads a `groups` field already on `BillingUsageRow`
  that astro-server has never populated (`GetBillingUsage` calls
  `V1.Usage.List` with no `billable_metrics`/`group_by`, so every row's
  `groups` is empty today), so the whole section renders nothing until the
  server sends a grouping: an empty breakdown reads as "you spent nothing".
  It appears the moment that lands, since the LLM stream needs no pricing
  step, `sum(cost_usd)` grouped by model is already dollars. Per-agent spend
  is left out entirely rather than shipped as a table that can only say "not
  available": the compute stream's `agent_name` grouping gives `cu_hours`, and
  turning those into dollars is the pricing step Astro doesn't do.
- The header, the charts, and the breakdown share one availability check and
  one pair of queries, resolved in a parent. Checking in each of them stacked
  three copies of the same "billing isn't available" notice.
- Every section on both pages loads as a skeleton shaped like its own
  content rather than a centered spinner, so the page holds its height.
- The resource-limit meters, the request-increase dialog, and the quota
  request history moved here unchanged from the old Quotas tab, in their own
  `ResourceLimitsSection`. They are "resource limits" on this page because
  "limits" already means the spend cap on the Billing page.

Daily usage keeps the per-metric charts from the old Usage tab (Compute Units
in CU-hours, LLM Usage in dollars) rather than one stacked dollar chart, since
stacking needs the same per-day compute pricing per-agent spend doesn't have.
Both charts read the same billing-period window as the header, so a series
starts on the cycle's own first day. Ticks carry a date, because a bare
day-of-month read as "1 2 3" once a window crossed a month boundary. One fill
opacity map drives both the header's legend swatch and the bars, so a colour
means the same stream in both places.

Dates on both pages render in UTC. The provider sends period boundaries as UTC
midnight, and a local-time render moves every one of them to the previous day
for anyone west of Greenwich.

## Migration

None. `useBillingBalances`, `useSetBillingUsageThresholds`, and the
`BalanceRow`/`toBalanceRow`/`totalCreditMoney` helpers are removed from the
client, since nothing on either page reads a per-credit breakdown or writes a
per-metric threshold anymore. The underlying `/billing/balances` and
`/billing/usage/thresholds` endpoints are untouched server-side, and `api.ts`
keeps its client methods for them.

## Follow-up (not in this change)

`BillingSpend` should carry `current_period_start`. The client reconstructs it
from `current_period_end`, which is right for a monthly cycle and guesswork for
anything else. The provider knows the real date.

Grouping "LLM Usage" by `model` in `GetBillingUsage`/`UsageData` is the cheap
half of the Agents/Models gap: astro-server would need to resolve the
customer's billable metric IDs (`V1.Customers.ListBillableMetrics`) and pass
`billable_metrics: [{id, group_by: {key: "model"}}]` to `V1.Usage.List`. The
client already reads `BillingUsageRow.groups` and renders the Models table the
moment the server sends it.

Per-agent dollars need a pricing step on top of that (`agent_name` group on the
compute stream gives `cu_hours`, not dollars) and a product decision on
whether "agent" means `agent_name` or `deployment_id`. This change assumes
`agent_name`, so spend survives a redeploy.
