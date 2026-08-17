# State usage spend in a spend message, not the credit-offset total

## Summary

A spend warning told the owner they had spent `$0.00` of the `$1.00` warning they
had just crossed.

Both spend messages state two numbers: the threshold and the amount spent
against it. The threshold came through correctly. The amount came from the
alert's `current_spend` property, and the message rendered `$0.00` while the
account's real usage was `$4.98`.

This is the same mistake the billing page carried. A spend threshold is measured
against usage **before** credit drawdown, so any account still on credit reads
zero for a figure that is climbing toward a warning it set itself. The page was
corrected to show usage. The notification was not, so the email and the page
disagreed on the same number.

The limit message shares the code path and had the same defect. It reports the
amount that caused a suspension, which makes a wrong figure worse there.

## Design

**The amount comes from the spend reporter, not the alert.** A new optional
`spendReader` on the Metronome webhook worker reads `Spend.UsageSpend`, which is
the same basis the provider measures a threshold against and the same figure the
billing page renders. The message and the page now cite one number by
construction rather than by coincidence.

Deriving it locally also removes a dependency on a provider field we cannot
verify. `properties.current_spend` is documented, and it did not arrive
populated; nothing in the alert payload reports the gross figure the alert
actually evaluated.

**Units convert once, at the edge.** Spend arrives scaled to the currency it
names, while a threshold arrives in the provider's minor units, and one message
carries both. The conversion happens where the two meet, so the pair cannot
render a hundred-fold apart.

**A failed read retries rather than guessing.** A threshold crossing fires once.
Stating a wrong amount is worse than stating a late one, so a read error returns
and the webhook job retries. Falling back to the reported figure would
substitute exactly the number that reads zero.

Two cases keep the provider's figure, because it is all there is: a backend that
reports no spend at all, and an invoice carrying no usage lines. The second is
self-contradictory (the alert fired on usage) and logs a warning.

## The dedupe test asserts what the unique tag actually covers

The Metronome redeliveries in `e2e/billing_webhook_dedupe_test.go` now vary
`CurrentSpend`. `InsertOpts` hashes only the `river:"unique"` event ID, so a
redelivery whose other fields moved must still collapse to one job. Three
identical inserts cannot tell that apart from whole-args equality, and this event
is exactly where the difference matters: the provider recalculates the amount
between attempts, so the sibling fields do drift in practice.

That distinction is the point of the test. Its Stripe counterpart already varies
the hosted invoice URL for the same reason.

## Migration

None. No payload property is added or removed, so the Novu workflow schemas are
unchanged. `billing.spend_warning` and `billing.spend_threshold` state a correct
`spent` value from the next crossing onward.
