# Let an account set its own spend warning and limit

Stacked on the read path. Second step of customer-set spend controls.

## Summary

`PUT /accounts/:account/billing/spend/thresholds` sets or clears both controls.
A null clears that one. PUT rather than PATCH, because an omitted field would be
ambiguous between "leave it" and "remove it", and removing a spend limit by
accident is the expensive direction.

## Design

Metronome remains the only store. Three endpoint details shape the
implementation, each found by reading the API reference rather than assumed:

**There is no edit.** Changing a threshold is an archive followed by a create,
and the archive must pass `release_uniqueness_key`. Without it the replacement
collides with the alert it replaces and the customer's new number never takes
effect.

**Evaluation is deferred.** `v1/customer-alerts/reset` runs after every write.
Without it an owner who raises a limit above current spend stays suspended until
Metronome next evaluates on its own.

**A create without `customer_id` applies to every customer.** The alert carries
one, and a `uniqueness_key` of `astro:spend_<kind>:<customer-id>` scopes the
write-side handle so a repeat create 409s instead of stacking a second alert.

Setting the number already in force is a no-op. Every settings save re-sends
both values, and rewriting an unchanged threshold would archive a live alert and
leave the account briefly uncapped.

### Rejected at the edge

A negative threshold fires the moment it exists, which reads as an outage rather
than a control the owner chose. A warning at or above the limit never fires on
its own, because the limit suspends the account first, so it is silently useless.

## Not yet wired

The webhook still treats every `spend_threshold_reached` the same, so a warning
currently gates like a limit. Splitting them is the next step, and until it lands
the warning should not be offered in the UI.

## Migration

None. No schema change, and no account has thresholds until it sets them.
