# Let an owner see and set spend controls

Final step of customer-set spend alerts and limits.

## Summary

A "Spend controls" card on the billing page shows what the account has spent this
period and the two thresholds it set on itself. Both are editable in one form.

## Design

The card sits above the billing tabs, next to the payment method, because both
are account-level rather than per-view.

**Units are the trap.** The provider stores USD cents, and the form edits
dollars. Showing cents would read as a number 100 times too large and invite a
limit set 100 times too high, so the conversion is pinned by a test in both
directions.

**Empty clears; zero caps.** An empty field removes the threshold. Sending zero
instead would cap the account at nothing and stop every agent it has, so the two
are deliberately different inputs and a test holds them apart.

**Untouched fields show the provider's value.** Local state starts null rather
than seeded from the response, so a field reflects what actually fires until its
owner edits it.

The two edge rejections mirror the server's, so the owner is told why rather
than getting a 400: a negative amount fires the moment it exists, and a warning
at or above the limit never fires because the limit suspends first.

The card renders nothing when the backend reports billing unavailable. A form
for settings that cannot be saved is worse than no form.

## Migration

None.
