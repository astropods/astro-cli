# Cover webhook redelivery in the billing signal tests

## Summary

Two properties of the billing signal path had no test, and both fail quietly
rather than loudly.

A provider redelivers webhooks. Entering dunning must keep the timestamp from
the first delivery, because the grace window is measured from it. Resetting it
on each redelivery walks the suspension deadline forward and the account keeps
running unpaid. `SetDunningSince` guards this with a `COALESCE` that prefers the
existing value, and nothing asserted that shape.

River collapses repeated webhook jobs by provider event id. An event that
arrives without an id must skip dedupe instead of hashing to a shared key, or
two unrelated events collapse into one job and the second signal is lost.

## Design

The dunning test asserts the full
`COALESCE(account_billing_status.dunning_since, EXCLUDED.dunning_since)`
expression rather than that the column is written. Matching only the column name
would still pass if someone simplified the statement to `EXCLUDED.dunning_since`
alone, which is the exact change that breaks the grace window.

The dedupe test covers both webhook arg types in both directions, since the
id-less case is the one where the safe choice is to double-process:
`ApplySignal` is idempotent, so a duplicate job converges, while a collapsed job
drops a signal outright.

Also removes two comments that described the rollout state of Metronome's
resolved notifications instead of the code.

## Migration

None. Tests and comments.
