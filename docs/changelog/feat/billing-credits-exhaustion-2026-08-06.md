# Credit exhaustion and the pay-as-you-go transition

## Summary

Signup provisioning grants an account its free credit, but nothing happened when
that credit ran out: usage kept accruing against a zero balance, nobody was
told, and there was no way to move an account onto paid usage. Metronome's
guidance is that usage accrual never stops on its own — the balance-exhaustion
webhook is what an integrator is expected to act on.

This closes that loop. A spent balance stops a free account, notifies its
managers, and surfaces a banner; adding a card lifts the stop and the account
bills pay-as-you-go from then on.

## Design

**Exhaustion gates only the card-less.** The correctness trap here is treating a
spent balance as a suspension reason on its own. Every paying account has a spent
balance by definition — that is what pay-as-you-go means — so gating on the
balance alone would stop customers the moment they became customers. The state
machine therefore reads two facts, not one:

```
credits_exhausted && !has_payment_method → suspended (credits_exhausted)
```

`has_payment_method` is the whole of the "shift them to pay-as-you-go" step.
There is no second package, no contract transition, and no plan record: the
account stays on the same contract and the same rates, and the card is what
decides whether an empty balance is a wall or a bill. Alise confirmed a package
change is not required for this, and one package keeps the free and paid paths
from diverging.

A paying account is still gated by payment collection — a failed charge enters
dunning exactly as before. Exhaustion is ordered above dunning in the state
machine so a card-less account reads "add a card" rather than a payment error it
cannot act on.

**Two new facts, both latches.** `credits_exhausted` is set by the Metronome
`alerts.low_remaining_contract_credit_balance_reached` webhook. `has_payment_method` is
written synchronously by the card save/remove handlers, because it must take
effect on the request that saves the card — the user is watching. Both route
through the existing `ApplySignal` funnel so writing a flag and recomputing the
cached status stays a single code path, and both reconcile workloads afterward,
so adding a card resumes the account's deployments and removing the last one
stops them again.

**Nothing external clears the latch.** Alerts fire on threshold crossing.
Metronome's notifications are stateful (`OK` / `IN_ALARM`) and the docs
reference a resolved notification, but no `_resolved` event type appears in the
webhook catalogue and the create-notification API exposes no flag to enable one,
so this does not depend on receiving a recovery event. The automatic paths out
are adding a card, and our own credit grant: the provisioning worker clears the
latch after granting, which covers a re-grant. A top-up issued straight from the
Metronome dashboard leaves the latch set — clear it by hand (`UPDATE
account_billing_status SET credits_exhausted = false`) until the server has a
top-up path.

Reading the balance rather than latching it would remove that manual step
altogether, since `GetNetBalance` and the customer-alerts state endpoint both
report current truth. That is a larger change and is deliberately not in this
PR.

**The card fact is re-read where it decides something.** `has_payment_method`
defaults to false, so every account that vaulted a card before this shipped
would read as card-less and be suspended the moment its credits ran out —
stopping a paying customer and telling them to add a card they already have.
Rather than a one-time backfill that a later gap could reopen, the Metronome
worker resolves the card from Stripe before applying exhaustion, which is the
one signal whose outcome depends on it. A failed read stops the job rather than
falling through to a stale false.

The owner notification is gated on the same fact. The alert fires for every
account crossing zero, including pay-as-you-go ones it does not gate, so the
notice is sent only when the account is actually suspended for
`credits_exhausted` — read from the row, so an account stopped for a write-off
is not told to add a card either.

**Banner reads cached state, not the provider.** `GET /billing/status` returns
the cached row with no Metronome call, so it is cheap enough to sit in the app
shell and poll. It reports `credits_exhausted` and `has_payment_method` next to
the status so the client distinguishes "free credits spent" from "card declined"
without parsing the reason string, and falls back to generic copy for a reason
the build does not recognise rather than hiding a stopped account.

## Migration

`account_billing_status` gains `credits_exhausted` and `has_payment_method`, both
`NOT NULL DEFAULT false`; Atlas applies them from the declarative schema. Nothing
changes until Metronome is configured, since no webhook can arrive without the
alert.

Beyond the setup the provisioning change already needs:

1. A **Contract credit balance** alert in Metronome, applied to all customers,
   threshold `0`. This fires
   `alerts.low_remaining_contract_credit_balance_reached`. Metronome's
   and-commit variant is accepted too, since we issue no commits; the
   percentage and days-remaining variants are not, so picking one of those
   silently disables gating.
2. That event type **subscribed on the webhook endpoint**, and
   `METRONOME_WEBHOOK_SECRET` set in the environment.
3. A **`billing.credits_exhausted` workflow authored in Novu** with payload
   properties `account`, `ctaUrl`, `timestamp`. Without it the notification is a
   no-op; gating and the banner still work.

No backfill of `has_payment_method` is required. It starts false for every
existing account, but the exhaustion signal resolves the card from Stripe before
applying, which is the only moment the value decides anything.

## Known gap

Gateway usage bills on `cost_usd`, which the OTel exporter writes only when
Bifrost supplies `gen_ai.usage.cost`. Bifrost's serving allowlist and its pricing
catalog are separate: an unknown model is rejected outright, but an allowlisted
model that the pricing catalog has no entry for serves normally and emits an
event with no cost, which bills as zero silently. Every model observed in preview
has a non-zero cost, but preview has only exercised the Anthropic models — the
Bedrock non-Anthropic entries (Nova, Mistral, Pixtral, Qwen, Titan) are unproven.
Making the exporter log and count a missing cost is an astro-infra change and is
not in this PR.
