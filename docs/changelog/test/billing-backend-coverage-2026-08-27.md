# Close backend billing coverage gaps across card signals, spend, and providers

## Summary

A coverage sweep scoped to backend billing, looking for gaps that would
actually catch a regression, not ones that would pad a percentage. Found six
areas with no test at all, several guarding transitions a comment already
flagged as easy to get backwards: `applyCardSignal`'s status transitions, the
billing spend handler's response assembly, Metronome customer dedup, the
pay-link staleness comparison, and the Stripe provider's setup-intent and
invoice-collection paths.

## Design

**`applyCardSignal` (`handlers/apply_card_signal_test.go`).** Untested
despite driving two account-status transitions from a single signal: adding a
card to a credits-exhausted account must resume it and re-derive its gateway
budget without also charging it (`collectAfterCard` only fires for a
non-Active result), while removing the last card from an exhausted account
must re-gate it, since otherwise the account keeps running on credits it no
longer has a way to pay for. Also covers that a card change during active
dunning still enqueues a same-window collection attempt even when the status
itself doesn't change, and that a nil status store is a no-op rather than a
nil-pointer panic.

**Billing spend handler (`handlers/billing_spend_handler_test.go`).** The
response-assembly closure that maps provider data onto the fields the client
renders had no test exercising it end to end. Added the normal case, an
uncovered-contract failure that must stop the request with a server error
rather than a 200 with an empty plan, a covered contract still succeeding
normally, and a failed threshold read not hiding the spend data, since that
read is explicitly best-effort.

**Metronome customer creation (`internal/billing/metronome/create_customer_test.go`).**
No test guarded against duplicating a customer: creating a second Metronome
customer for an account that already has one would split its usage across
two records instead of accumulating on one. Covers alias reuse, creating with
both the account ID and Bifrost customer ID as ingest aliases when neither
exists, and omitting the Bifrost alias entirely when unset rather than
sending an empty string Metronome would reject (and that would collide
across every account with none set).

**Metronome usage ingest (`internal/billing/metronome/ingest_usage_test.go`).**
Covers batch chunking at Metronome's ~100-event ingest limit, that an
exactly-one-batch call is one request and not a second, empty one, and that
zero events sends no request at all. The Metronome SDK's
`V1UsageIngestParams.MarshalJSON` sends the usage slice as a bare JSON array,
not `{"usage": [...]}`; the mock server in this test decodes accordingly.

**Pay link staleness (`internal/billing/pay_link_test.go`).** The doc comment
on `ClearStalePayLink` already calls out that its comparison must stay "clear
when different", not "clear when same", since keeping a stale link risks
charging against the wrong invoice. Added the inverted-comparison case
alongside the basic upsert, and that an event with no invoice URL clears the
link rather than leaving it, the documented safe direction for an unnamed
invoice.

**Stripe provider (`internal/payment/stripe_test.go`).** `ConfirmSetup`'s
happy path retrieves the setup intent, detaches every other saved card, sets
the new one as default, and returns it; `detachCardsExcept` is unexported and
only reachable through this call, so it's exercised through the same path
production uses. Also covers the three rejection cases (wrong customer,
unsucceeded status, missing payment method), that
`CollectOpenInvoices` counts a card decline as skipped rather than failed,
and the no-card-on-file and remove-all-cards cases for `DefaultCard`/`RemoveCard`.

## Migration

Test-only. No behavior change.
