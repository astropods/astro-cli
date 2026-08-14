# Show an account its spend and its own thresholds

First step of customer-set spend alerts and limits: the read path.

## Summary

An account could not see what it was running up. `CustomerSpend` has always
computed current-period spend and credit remaining from Metronome, but only
astro-queen read it, so the number existed and no customer could reach it.
Nothing exposed spend controls either, because none existed.

## Design

`GET /accounts/:account/billing/spend` returns current spend, credit remaining,
and the account's own warning and limit. They ship in one response because a
threshold is meaningless without the number it is measured against.

### Metronome is the only store

The thresholds are Metronome alerts and nothing mirrors them here, matching how
balances and credits already work. Three facts made that possible, each checked
against the live environment rather than assumed:

- `v1/customer-alerts/list` returns a customer's alerts with a per-customer
  `customer_status` of `ok`, `in_alarm`, or `evaluating`.
- The org-wide backstop appears in that same list.
- The SDK's alert type carries no `customer_id`, so the alert name is the only
  discriminator available.

So the two thresholds are named `astro:spend_warning` and `astro:spend_limit`,
the same trick contracts already use with `uniqueness_key`. Anything else in the
list is not the customer's, which keeps the org-wide backstop from being
reported as a limit the account never chose.

Absence is a `Has` flag, not a zero. Zero is a threshold a customer could
legitimately set, and rendering it as one would tell an unconfigured account it
had capped itself at nothing.

A threshold read that fails logs and returns the spend anyway. Losing the number
would remove the only thing a customer needs to choose a threshold in the first
place.

## Migration

None. No schema change, and no account has thresholds until it sets them.
