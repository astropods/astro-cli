# Bound the value a customer can set a threshold to

## Summary

`SetBillingSpendThresholds` validated two things: the value is not negative, and
the warning sits below the limit. Both pass for `1e308`.

A limit that high never fires. The account is uncapped, the settings page shows
a cap, and nothing distinguishes the two states. It is reachable from the public
API with one request, and it is the one control standing between an account and
unbounded spend.

`SetBillingUsageThresholds` had the same two checks and the same gap.

## Design

**One bound for both, in the caller's unit.** Spend thresholds are USD cents.
Usage thresholds are CU-hours or USD, depending on the metric. `1e9` is absurd in
all three and finite in all three, so a single constant covers every writer
without pretending the units are the same.

**A typo guard, not a product ceiling.** What a customer should be allowed to
cap themselves at, and where `handlers/quota_increase.go` should take over, is a
product decision and is still open. This change refuses values no answer to that
question would allow.

**The bound is inclusive.** The largest legitimate value has to survive a guard
whose purpose is catching a slipped decimal point.

**Null still clears.** A `PUT` with a null field removes that control. Reading
null as a value to bound would make a threshold impossible to remove.

## Migration

Nothing to do, and nothing here makes an uncapped account capped.

The bound applies to writes, so a threshold already above it stays in force until
someone re-saves that control. Two writers exist: provisioning, which seeds $20,
and the settings form.

Most accounts have no limit to bound. Of the 63 provisioned preview accounts, 61
carry no `astro:spend_limit`, and 2 of those carry a warning with no limit behind
it, so the warning fires and nothing stops the spend. The $20 default landed on
2026-08-20 and `seedSpendLimit` runs only for an account that has never been
provisioned, so every account created before that date is uncapped.

Re-running provisioning does not fix that, by design: the seed is skipped for a
provisioned account so a re-provision cannot reimpose a cap the owner cleared.
Nothing records whether an account never had a limit or removed one on purpose,
so a backfill needs that distinction before it can be safe. That decision is
still open.

## Note

Metronome publishes no maximum for an alert threshold, so this refuses at our
edge rather than relying on the provider to. That matters more than it looks:
changing a threshold archives the old alert before creating its replacement, so
a value the provider refuses would leave the account with no threshold at all.
