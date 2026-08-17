# Show spend controls the number they are actually measured against

## Summary

The spend controls card showed `$0.00 this period` for an account with real
usage, and a second bug meant any non-zero figure would have printed as a
hundredth of itself.

Both are visible on the preview billing page today.

## The number was the wrong one

`CustomerSpend` reads `CurrentSpend` from the latest draft invoice total, which
is net of credit drawdown. That is the right number for "what will I be charged".
It is the wrong number to put beside a threshold, because the provider's spend
threshold notification triggers on usage-based spend **prior to** commit and
credit drawdown.

An account on signup credit therefore reads `$0.00` for the whole period while
its usage climbs toward a warning it set itself. The controls look inert right up
until one fires, and the number the customer set a threshold against is not the
number shown to them.

`UsageSpend` sums the invoice's `usage` line items, which is the same basis the
provider measures. Credit drawdown arrives as its own negative line, typed
`applied_commit_or_credit`, so the two are separable without inspecting names or
signs. `HasUsageSpend` distinguishes an invoice with no usage from one totalling
zero, because reporting zero as a fact would claim the account spent nothing.

The card now reads `$2.76 used this period`, and the explanatory copy states that
both controls measure usage before credit, so they can trigger while the bill is
still zero.

## The scale was wrong too

One response carries money in two units. Spend and credit are converted to the
currency named alongside them, because astro-queen renders them directly. The
thresholds are the provider's raw cents, because that is what the write path
sends and what the provider stores.

`SpendControls` applied one conversion to both, dividing spend by 100 a second
time. Thresholds rendered correctly; spend rendered at a hundredth. A `$12.34`
period read `$0.12`. It went unnoticed because every preview account is on credit,
so the value under test was always zero.

The two units now have separate helpers with the reason stated at each, and a
regression test renders a spend and a threshold together and asserts they agree
on scale.

Leaving the units mixed is deliberate for now. Converting the thresholds would
mean changing the write path and the provider call in the same PR as a display
fix, and the write path is correct as it stands.

## Migration

None. `usage_spend` and `has_usage_spend` are added to
`GET /api/v1/accounts/{account}/billing/spend`; no existing field changes shape
or unit.
