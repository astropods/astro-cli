# Let a recovered credit balance un-gate an account

## Summary

Credit exhaustion is a latch. `alerts.low_remaining_contract_credit_balance_reached`
sets it, `computeStatus` suspends every deployment on a card-less account that
carries it, and the only thing that clears it is `SignalCreditsGranted`, which
only the provisioning job emits. Metronome knows the moment a balance recovers.
We never heard about it, so an account whose credits were topped up outside that
job stayed suspended until an operator forced a provisioning re-run.

That is why the forced re-run exists. It is an operator button for a state the
system should leave on its own.

## Design

**The resolved alert is the missing clear.** Metronome emits a `_resolved`
event on the `IN_ALARM -> OK` edge, so the recovery signal is the same alert
crossing back. It maps to `SignalCreditsGranted` rather than a new signal
because the effect is already exactly right: clear the latch, recompute, and
let the status fall back to whatever the other flags say. A dunning failure or
a spend alert still holds the account suspended on its own terms.

**Both contract-credit variants map, mirroring the `_reached` pair.** Metronome
documents the resolved event for `low_remaining_contract_credit_and_commit_balance`
only, but the switch is per-account and covers every threshold type once
support enables it, so the plain variant we are actually configured for will
fire too. Accepting both means the mapping does not depend on which alert type
we run, and changing that later stays a dashboard change rather than a deploy.

Nothing else was needed. The webhook handler has no event allowlist, and
`applyWebhookSignal` already reconciles workloads to the recomputed status on
every handled event, so the resume enqueues itself.

**No owner notification.** The resolved alert fires for every account crossing
back above the threshold, including pay-as-you-go accounts that were never
gated. `SignalCreditsExhausted` already suppresses its notification for those,
and the mirror guard would need the pre-signal status. The banner clearing and
the agents restarting are the observable outcome; a message saying so is not
worth reading the row twice.

**The spend-threshold latch gets the same treatment, via its own signal.**
`alert_active` previously cleared only on `SignalVoided`, so a spend suspension
had no exit short of voiding an invoice. `SignalRecovery` cannot be that exit,
and deliberately is not: a payment says nothing about period spend. The alert's
own `IN_ALARM -> OK` edge is the one event that does, so it maps to a new
`SignalAlertResolved` that clears the alert flag and nothing else. A write-off
or an open dunning cycle still outranks it.

Only the and-commit credit event is documented by name. The rest follow the
enum's `<alert_type>_resolved` form. A guessed name that turns out wrong falls
through to the existing unhandled-event log, which is exactly today's
behaviour, so the inference costs nothing if it misses.

## Migration

None in the code. The events only arrive once Metronome support enables
resolved notifications on the account, and until then the mapping is inert.
