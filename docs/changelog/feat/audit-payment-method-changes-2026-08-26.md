# Record card changes in the audit log

## Summary

Adding and removing a payment method left no audit entry. Reconstructing when a
card came off an account meant inferring it from the shape of a Metronome
invoice, which only narrows it to the hour and only works while the usage is
recent.

A card change decides whether an account can be charged at all, so it belongs in
the trail beside `account.delete` and `member.remove`.

## Design

Two actions, following the existing `<resource>.<verb>` pattern:

| Action | Written by |
|---|---|
| `payment_method.add` | `ConfirmPaymentMethod`, once the card is vaulted |
| `payment_method.remove` | `DeletePaymentMethod`, once the detach succeeds |

Both carry the brand and last four, as `Removed payment method Visa ending 4242`,
plus the actor, IP and user agent that `FromGinContext` already collects. Nothing
else about the card is recorded: the expiry identifies nothing a reader needs and
there is no reason to hold card data in a second place.

Removal reads the card before detaching it, because afterwards there is nothing
left to name. That read is best effort. A provider that cannot answer still
produces an entry saying a card was removed, since a gap in the trail reads the
same as nothing having happened.

Both entries are written after the provider call succeeds, so the log records
what happened rather than what was attempted.

The audit page needs no change. Its resource-type and action filters are
`SELECT DISTINCT` over the table, so `payment_method` appears on its own once
there are rows.

## Migration

None. Existing entries are unaffected, and no backfill is possible for card
changes that were never recorded.
