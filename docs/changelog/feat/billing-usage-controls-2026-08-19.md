# Let an account cap its own metered usage

## Summary

The unlimited plan rates every product at a zero multiplier. Priced spend is
therefore always zero, so a spend warning never notifies and a spend limit never
gates. An account on that plan has no working spend control of any kind, while
the compute and model usage behind it still costs real money upstream.

Quantity is rated by nothing. A threshold on the billable metric still moves
when the rate does not, so the controls are expressed in the quantity each meter
counts rather than in money.

## Design

**A usage cap is the account's own control, not a provider verdict.** That is
what makes it the one gate the unlimited plan can carry. The credit-exhaustion
exemption exists because the provider must not stop an account the plan promises
never to stop. A cap the owner set is the opposite: honouring it is the promise.
So `SignalUsageLimit` is deliberately not exempted, and the exemption stays
scoped to `SignalCreditsExhausted`.

**The cap has its own latch and its own exit.** `usage_limit_active` is separate
from `alert_active` because the two do not clear together: a spend alert ends on
the provider's spend edge, a usage cap ends on the quantity edge, on the owner
changing the number, or on the billing period rolling. Sharing one column would
make either exit lift both.

**Ranked below the provider's gates and above dunning.** A write-off, a spend
alert, and spent credits all name themselves first, because each is a real
collection problem and the cap is a preference. Above dunning, because an
account stopped by its own number should be told that rather than sent to
replace a card that is working.

**The warning cannot gate, by construction.** `isUsageWarning` short-circuits in
the worker before the signal logic, exactly as `isSpendWarning` does. The two
controls are the same provider primitive at different numbers, so only the alert
name distinguishes them, which makes the name load-bearing in the same way the
spend names already are.

**Metric ids are resolved, not configured.** They differ per environment, so
they are looked up by matching the billable metric's event-type filter against
the event types the meters emit (`deployment_compute_usage`,
`ai_gateway_llm_usage`). Those strings are fixed by this repo rather than by
dashboard state, and the lookup is cached for the process.

**Recovery does not wait on the provider.** An account suspended by its own cap
has nothing running to produce the usage event that would re-evaluate the alert,
so the write endpoint clears the latch and reconciles workloads directly. The
resolved edge is handled too, which is what returns an account at the start of a
new billing period.

**One control, not two.** The caps join the existing spend warning and limit in
a single card rather than a second one beside it. The billing page already asked
"warn me at" and "stop agents at"; asking again under a different heading reads
as two settings for one thing. The spend row is omitted on a plan that rates
everything at zero, because a field that provably cannot fire is worse than no
field.

**Each metric keeps its own unit.** Compute counts CU-hours; the gateway counts
US dollars of upstream model cost. They do not share a unit, so the controls are
per metric and the unit travels with the numbers through the API, the settings
page, and the CLI. Converting both to one figure would mean duplicating rates
that live in the provider.

**One latch for every limit the account set, in either unit.** The spend limit
is a number the owner chose, so it belongs on the same latch as the quantity
caps rather than on `alert_active`. It previously shared the latch with an
operator's org-wide alert, which made its reason `balance_alert` and its fix
"contact support", and left it with no exit at all: the resolved edge only
arrives when period spend falls back under the threshold, and spend does not
fall within a period. `alert_active` now carries operator alerts only, where
naming support is correct. The webhook keeps the spend-shaped message for a
spend cap, because one latch does not mean one unit.

**Lifting the latch is measured, not assumed.** A raised or removed limit
restarts the account only when every limit it set is above what the period has
actually counted. Reading the provider's own `in_alarm` flag cannot answer that
straight after a write: the alert was archived and recreated, so it still
reports on the number it replaced. Lowering a cap under current usage therefore
leaves the account stopped, where an unconditional lift restarted it for exactly
as long as the next usage event took to stop it again. An account that is not
latched skips the work entirely.

**An account with no contract stops.** Nothing it runs can be rated, so
`not_provisioned` outranks every other reason: the others describe a problem
within billing, and this is the absence of billing. Provisioning writes it, on
success and on failure, because that is the run that either establishes the
contract or does not. The billing read reports the gap as an error rather than
rendering a "not set up" plan card, but writes nothing: a GET that suspends an
account is a surprise, and two concurrent renders would both enqueue the stop.
Every account is put on a package at signup, so dressing the fault as a plan
state would hide it.

**Coverage has one definition, in one call.** `CustomerPlan` reports the plan
and whether any contract covers the customer, because those are different facts
and only the second one stops an account. A contract on a package this build
does not recognise renders without a plan label and logs; inferring "no
contract" from an unrecognised plan would suspend a live account over
configuration drift, and it would disagree with what provisioning concluded
about the same account. The read and provisioning ask the same method, so neither
can hold its own idea of covered.

**Metric ids cache only a complete answer.** The lookup runs on the write path,
so caching a transient failure would refuse every later cap write for the life of
the process. A partial list is the same failure at metric granularity: a metric
that does not exist yet, or was recreated, would stay missing until a restart.

**A quantity threshold does not round.** The webhook envelope rounds amounts to
minor units, which is right for money and wrong for a cap counted in CU-hours or
dollars of model usage. The unrounded value travels beside it, so a cap of 25.5
reaches the owner as the number they set.

**The spend messages name their window.** `billing.spend_threshold`'s email
already referenced a period the payload never carried, so the line rendered
blank. `CustomerSpend` returns the period end and the worker already calls it for
the spend figure, so carrying it costs no extra read. An absent period drops the
key rather than sending an empty string, which would render an empty line instead
of no line.

**A failed lift is reported, not swallowed.** Raising a cap is the only thing
that clears an account stopped by its own limit: the alert is archived and
recreated on every write, so the provider emits no resolve edge of its own. When
that lift fails the controls still saved, so the write returns 200 with
`limit_lift_failed`. The client warns instead of reporting success and keeps Save
enabled, and a second Save re-sends the same numbers to retry the lift.

**The CLI moves with it.** `modules/astro-cli` is bumped to the merge of
astropods/astro-cli#17, which adds `ast billing get`, `status`, `invoices`, and
`set`. `set --metric compute|gateway` writes the quantity caps this change adds,
so the pin and the endpoint land together.

## Migration

`account_billing_status` gains `usage_limit_active` and `not_provisioned`, both
defaulting to false. No backfill: an account without a limit is unaffected, and
a covered account clears the coverage flag on its next provisioning run or
billing-page read.

Public docs gain a Usage limits page under Platform.

Two Novu workflows are required before the notifications deliver,
`billing.usage_warning` and `billing.usage_limit`. Both receive `account`,
`ctaUrl`, `metric`, `unit`, and `threshold`. A workflow that is missing or
switched off is reported by the delivery job rather than failing the webhook.

No limits exist until an account sets one, so nothing changes for an account
that does not.
