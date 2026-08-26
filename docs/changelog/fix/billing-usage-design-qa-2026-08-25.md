# Billing and Usage design QA

## Summary

Design QA on the Billing and Usage pages that shipped in
`billing-and-usage-ui-update-2026-08-24.md` returned a list of gaps between
the built pages and the design. Most are chrome: wrong copy, a card border
where the design has a hairline, a badge that does not match the one the
deployed agent cards use. Two are substantive. The invoice status column
showed the billing provider's own vocabulary, and the Usage page could only
ever describe the open billing period.

Two items in the QA list are not addressed here because the data behind them
does not exist yet. Both are called out under Not addressed.

## Design

**The pages stop wrapping every section in a card.** Usage previously nested
a `This billing period` heading over a bordered card, then a second bordered
card around the chart. Both are gone. `UsageView` now lays its bands out in
one column separated by hairlines, which is what lets the period header, the
chart, and the quota grid read as one page rather than three tiles. Billing
keeps its cards, because there each one is a distinct object (the plan, the
payment method, the invoice list), but `PayAsYouGoCard` gains internal
structure: a hairline under the card header, and a tinted band around the
spend figure and its meter.

The meter itself no longer stretches. A `flex-1` track pushed the
`$20.00 spend limit` label to the far card edge, which read as a separate
right-aligned column rather than the end of the meter. The track is now a
fixed proportion of the row with the limit immediately after it.

**Invoice status is translated, not passed through, and payment is read
rather than assumed.** Metronome reports `DRAFT`, `FINALIZED`, and `VOID`.
The table rendered those verbatim, so an account holder saw `FINALIZED`, a
word that means nothing outside the provider's own model.

The obvious mapping is wrong, and worth spelling out because it is the kind
of error that reads as correct. Metronome's `FINALIZED` means it closed and
issued the invoice. It says nothing about collection, so mapping it to Paid
would tell an account whose card was declined that its invoice settled.

Payment state lives in one place, `external_invoice.external_status`, which
the downstream billing provider populates (`PAID`, `PARTIALLY_PAID`,
`PAYMENT_FAILED`, `UNCOLLECTIBLE`, ...). The invoices endpoint already
returns it: the handler passes the provider's own `[]metronome.Invoice`
straight through, and the SDK type tags its response metadata `json:"-"`, so
the field was reaching the client and simply going unread. `invoiceStatus`
now reads it:

- `DRAFT` reads as Pending, since the period is still accruing.
- `FINALIZED` reads whatever the external status reports, and reads **Issued**
  with a muted badge when there is none. That is the honest answer for an
  account whose downstream provider is not connected yet, where the field
  comes back empty for every invoice.
- `VOID` reads as Void, and an unrecognised status falls through with its raw
  value rather than being forced into one of the others.

`BillingExternalInvoice` is added to `lib/api.ts` to describe the field. No
server change was needed.

**Reading payment status means keeping it fresh.** Surfacing a payment outcome
turns a cache-staleness question into a correctness one. Saving a card
enqueues `billing.collect`, which charges the account's open invoices, so the
save is exactly the moment an invoice can go from failed to paid.
`useConfirmPaymentMethod` invalidated the payment-method and gating-status
queries but not the invoices query, which meant an account that fixed its card
kept reading `Payment failed` until the 60-second `staleTime` lapsed and
something happened to trigger a refetch. Under the old column that was
invisible, because `FINALIZED` made no claim about payment either way.

The mutation now invalidates the invoices query alongside the other two.

**There is no completion signal to wait for, and pretending otherwise is
worse than admitting it.** Two properties of the collection path rule out
timing the refetch:

- `collectAfterCard` only enqueues the job when the signal is a card add *and*
  the account is not already active. A card saved on a healthy account queues
  no collection at all, so any delay scheduled on every save is mostly waiting
  for nothing.
- `BillingCollectWorker` deliberately does not write the status change. Its own
  comment records why: the change rides the resulting provider webhook so that
  a payment succeeding outside our window takes the same path. That means the
  job returning tells us nothing about whether the invoice's `external_status`
  has flipped yet. The event that matters is the provider processing a webhook,
  on its own clock, which nothing in this system observes.

So the refetch on save reads whatever is true at that moment, which is often
still the old status.

**Riding a signal that already exists.** The transition is unobservable
directly, but it is not unobservable entirely. Collecting an invoice clears
dunning, and clearing dunning changes the account's gating status.
`useBillingStatus` already polls that every 60 seconds, because a
webhook-driven suspension has the same problem in the other direction.

`useWatchInvoicePayments` invalidates the invoices list when that polled status
moves under it. This is tied to an observed consequence of the payment rather
than to a guessed delay, it stops on its own because it fires on a change
rather than on a timer, and it adds no network traffic: the poll it watches was
already running.

The signature includes `reason`, not just `status`. `computeStatus` returns one
reason by precedence, so a payment clearing dunning while a spend limit still
holds leaves the account suspended and moves only the reason. Watching status
alone would miss it.

**A gate that outranks dunning masks the payment in both directions.** If a
balance alert or an unprovisioned contract is what the account is suspended
for, dunning never surfaces as the reason, so nothing about the gating status
changes when the charge lands. There is no signal left to ride, and
`refetchOnWindowFocus` is off app-wide, so a Billing page left open would never
recheck.

`useBillingInvoices` therefore rechecks itself, but only while it is holding a
claim that can still move. The predicate is framed around what has settled
rather than what has failed, because a finalized invoice with no reported
outcome is not a resting state: it is where every invoice sits between
Metronome closing it and the provider reporting a result, which is the ordinary
path to Paid. Treating only an explicit failure as unsettled would have watched
for the recovery case and missed the common one.

Settled means the provider will not move off it on its own: `PAID`, `VOID` and
`DELETED` are final; `UNCOLLECTIBLE` is a write-off someone chose; `SKIPPED`
and `INVALID_REQUEST_ERROR` are the provider declining to bill at all, which no
amount of waiting changes. Everything else on a finalized invoice is still in
flight.

**In-flight is not the same as imminent, so the watch is capped.** A declined
card nobody fixes stays `PAYMENT_FAILED` for weeks, and a part payment stays
`PARTIALLY_PAID`. Waiting on either indefinitely is expensive in a way worth
naming: `Customers.Invoices.ListAutoPaging` is called with only a customer id
and no date bound, so every recheck walks that customer's entire invoice
history. Twice a minute, for as long as a tab stays open, on an account that
will never resolve.

Rechecking therefore stops five minutes after the page mounts. That is a cap on
spending, not a prediction of when the payment lands, which is the distinction
that matters: nothing here claims to know the timing, it just stops paying to
ask. Reopening the page starts a fresh window, and reopening the page is the
right trigger for looking again anyway.

One more condition keeps this off pages that have nothing to wait for.
`external_invoice.billing_provider_type` is the tell that an outcome is coming
at all: with no downstream billing connection the external status is empty on
every invoice permanently, so an empty status by itself would mean waiting
forever on nothing. Rechecking requires a provider to be present. In an
environment with no connection the interval never starts.

The interval is a function of the query's own data, so it begins when an
in-flight invoice appears, stops the moment that invoice settles, and never
runs for an account whose invoices have all resolved.

## A local billing provider

None of the invoice payment states above can be reached in any environment we
run. `external_invoice.external_status` is written by Metronome's own Stripe
connection, and that connection is not configured, so the field comes back
empty on every invoice. Paid, Payment failed, Partially paid and Uncollectible
were all written without ever being seen, and a closed billing period in the
new picker needs a finalized invoice from a prior cycle.

`BILLING_PROVIDER=fake` selects a third backend that answers every billing read
from canned data: one invoice per payment outcome, a rated daily series with a
gap in it, and credit partly drawn down so the card shows both an applied
amount and a remaining balance.

It deliberately models nothing that has no real counterpart. The Usage page's
Models breakdown is the case that matters: it reads a `groups` map off the
usage rows, and the provider call passes no `group_by`, so Metronome never
returns one. That section renders for nobody today. Populating it in the fake
would have put a table on the page that no account can see, so the fake leaves
it empty and the section stays hidden, exactly as it does in production. Spend
thresholds are held in memory and their `in_alarm` is evaluated locally against
the period's usage, which is what makes the paused-agent badge and the resume
link reachable.

The provider seam took this almost unchanged: `BillingProvider` is nine
methods, and everything these pages need beyond it is an optional interface the
handler type-asserts, so `noop` was the template. Two things did need changing.
`resolveBillingCustomer` gated on the literal `"metronome"`, which would have
made every read report unavailable; that is now
`config.BillingBackendHasCustomers`, which names the property being tested. And
the `StatusStore` was constructed only for Metronome even though it is
DB-backed and needs no provider, so the fake would have had a pass-through gate
and a hardcoded active status.

The fake derives its customer id from the account and persists nothing.
Reusing Metronome's stored column was the first approach and was wrong: local
`DATABASE_URL` points at a shared dev database, so it would have left a
`fake-cus-` id on the Metronome column of every account browsed, breaking real
billing reads for that account after switching back.

Stripe needs no fake. Local keys are already `sk_test_`/`pk_test_`, the same
mode preview uses, so the card flow runs for real against test mode.

**The Stripe webhook works on the fake, and it should.** The route and its
worker were gated together with Metronome's, but the two resolve accounts
differently: `MetronomeWebhookWorker` looks up by Metronome customer id, while
`StripeWebhookWorker` looks up by Stripe customer id, which the payment
provider persists whatever the billing backend is. Nothing about the Stripe
path needs Metronome.

So the billing worker block now runs for the fake as well, because its status
store is the same DB-backed one. Dunning, suspend/resume and the dunning-grace
sweep are all real there, which means `stripe trigger` against test mode drives
an actual gate transition locally. Two things stay behind the narrower
Metronome check: its own webhook worker, and provisioning, which writes a
customer id keyed on the backend and would otherwise create one every hour and
store none of them.

**`scripts/dev.sh` starts `stripe listen` itself** when the CLI is installed
and logged in, `STRIPE_SECRET_KEY` is set, and the backend registers the route.
Anything else skips silently, because this is a convenience and must not stop
the server booting. `scripts/local-dev.sh` runs the same script, so both entry
points get it from one place.

Every value is resolved the way the server resolves it, environment first and
`.env` only as a fallback. That is not cosmetic: `godotenv.Load` skips a key
already exported, so a `PORT=9000 moon run astro-server:dev` that read the port
from `.env` alone would forward to a port nothing is listening on. The same
applies to `BILLING_PROVIDER`, where the two could disagree about whether the
route exists at all.

The signing secret comes from `stripe listen --print-secret`, since the
listener is what generates it, and exporting it is what delivers it to the
server for the same godotenv reason.

The listener is pinned to the server's own `STRIPE_SECRET_KEY`, which is the
part that is easy to get wrong. `stripe listen` otherwise authenticates with
whatever account `stripe login` last saved, a credential entirely separate from
the server's. Those can be different accounts, or the CLI can be on live while
the server is on test. Nothing would look broken: `--print-secret` returns that
other account's secret, so signature verification passes, and the forwarded
events are for customers the server has never seen. They resolve to nothing and
the webhook appears silently dead. The key is passed through `STRIPE_API_KEY`
rather than `--api-key` so it does not sit in the process list.

Only a test key is forwarded at all, and that is expressed as an allow-list
(`sk_test_`, `rk_test_`) rather than a list of live shapes to refuse. Live keys
come in more than one form, `sk_live_` and `rk_live_`, so refusing the ones you
thought of leaves the restricted variety forwarding real webhooks into local
dev. Naming the two safe shapes fails closed: a publishable key, a legacy
prefix, or anything unrecognised skips instead of being trusted.

Teardown is deliberately not a trap. The script ends in `exec air` so air
receives signals directly and can kill its own child, and a trap hop would
regress that. Ctrl-C reaches the listener anyway, because a background child of
a non-interactive script shares its process group. For the case that does leak,
a killed task or a crash, the next run clears any listener on this webhook path
on any port, so a run that changed `PORT` cannot leave one behind and two
listeners cannot deliver every event twice.

## Follow-up worth taking separately

The invoices endpoint walks the provider's full invoice list on every call,
with no date or count bound. That is the reason rechecking needs a cap at all,
and it is a cost the first page load already pays. Bounding that list server
side would be worth doing on its own.

The table also moves to the design's columns (Date, Invoice number, Amount,
Status) inside a bordered card, and the download control becomes an icon in a
fixed-width slot rather than an icon-plus-label that changed the column width
between rows. Its tooltip now covers the enabled case too, since an
icon-only button has nothing else naming it.

**Usage can look at a closed period.** The page derived one window, the open
billing period, from `useBillingSpend`, and every query hung off it. A period
picker now sits in the page header, listing the open period plus every
distinct period the provider has already cut an invoice for.

Picking a closed period re-points the two windowed queries at it. It cannot
re-point `useBillingSpend`, which has no window parameter and always answers
for today, so the header changes what it reads instead of quietly showing a
current-period number under a past-period label:

- The open period keeps reading `usage_spend`, the server's own gross figure,
  and keeps the spend-limit meter.
- A closed period totals its own daily breakdown, and drops the meter. A
  spend limit is a live cap on the open period; drawing a closed period
  against it would invite the reader to act on a bar that can no longer move.

**Chart chrome comes from the Insights page.** The daily spend chart carried
its own copies of the grid, axis, and tooltip styling. It now imports
`GRID_PROPS`, `yAxisProps`, and `SeriesTooltip` from
`components/activity/chart-chrome`, which is where the Insights and activity
charts already get theirs, so the two cannot drift.

`SeriesTooltip` gains one option. It drops zero-valued series, which is
correct for a stacked chart where every series carries a key on every row and
without it the tooltip fills with rows reading nothing. On a single-series
chart the zero is the reading, and suppressing it means an account with no
spend yet gets no tooltip anywhere. `includeZero` opts into keeping it, and
defaults off so the existing callers are unchanged.

**Field messages move next to their field.** `ManageLimitsDialog` put both
inputs' helper text under their inputs and then collected any validation
error at the bottom of the dialog, below both. The error naming the alert
threshold could therefore sit two controls away from the input it was about.
Each field now reads label, then what the field does, then the input, then
anything wrong with it, and validation is split per field rather than shared:
a malformed alert reports on the alert, and the cross-field conflict reports
on the alert because that is the value the message asks the reader to change.
The conflict is only evaluated once both fields parse, so a half-typed `-`
does not also claim to be above the spend limit.

Both amount inputs also share one `0.00` placeholder. They previously read
`No alert` and `No limit`, which described the state rather than the expected
input and made two identical controls look like different kinds of field.

**Quotas.** The section was titled `Resource limits` with a sentence of
explanation under it and a rule under every row. It is now titled `Quotas`
with no subtitle and no rules. The `Quota increase requests` heading and its
empty table are hidden entirely when an account has none, instead of standing
as a permanent heading over `No quota increase requests.`, which reads as
something the account has to act on.

## Not addressed

**A tokens sub-line under Models.** The QA asks for a raw-quantity line under
each stream's dollars, `219.05 CU-hours` under Compute and `1.24M tokens`
under Models. Compute is done: its billable metric aggregates `cu_hours`, so
the quantity is already in the usage rows.

Models is not, because no token quantity reaches billing. The gateway usage
event carries `total_tokens` in its properties, but the `LLM Usage` billable
metric aggregates `cost_usd`, so the usage endpoint returns dollars for that
metric and nothing else. Token counts exist on the observability side, which
is a different provenance from a billing figure and not somewhere this page
should reach. Filling this in means adding a token-aggregating billable
metric in Metronome and surfacing it, which is a separate change.

**A Paid label for the common case.** With the downstream provider
unconnected, `external_status` is empty and every closed invoice reads
Issued. That is accurate but not informative. Paid only starts appearing once
that provider is wired up, which is tracked separately.

**Opening the invoice from the pay-as-you-go card.** The button is now
singular, `View invoice`, since the row it sits on names one. It still scrolls
to the invoice list rather than opening the invoice. The row names the
upcoming invoice, which is a draft, and Metronome renders no PDF for a draft,
so there is nothing to open. If the intent is to jump to that invoice's row
rather than the top of the list, that is a small follow-up.

## Migration

None. No API, storage, or configuration changes.
