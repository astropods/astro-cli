# Let Queen re-run provisioning to clear a stale exhaustion gate

## Summary

An account gated by `credits_exhausted` could not be un-gated from Queen. The
latch is cleared by exactly one thing, `SignalCreditsGranted`, which is applied
at the end of the provisioning job. `RetryBillingProvision` refused to enqueue
that job whenever `billing_provisioned_at` was set, and every account able to
hold a latch is by definition already provisioned. So the one action that clears
it was unreachable for exactly the accounts that needed it.

This matters when credit is added outside Astro. An operator granting credit in
the Metronome dashboard raises no signal, so the account keeps a spent-balance
latch while holding a balance, and stays gated. `ForceBillingResume` does not
help: it restarts workloads but deliberately leaves the status alone, so the
402 gate stays up and the next recompute stops them again.

## Design

`RetryBillingProvisionRequest` gains `force`. Set, it skips the
already-provisioned guard; unset, behaviour is unchanged. Queen sends it when
the account is already provisioned, so the existing button covers both cases
instead of being disabled on precisely the accounts an operator needs it for.

Re-running is safe rather than merely tolerable. `ProvisionCustomer` lists
covering contracts first and returns without writing when one exists,
`MarkBillingProvisioned` is an idempotent write, and `SignalCreditsGranted`
against an account with no latch is a no-op update whose recompute yields the
same status. Nothing is created twice.

This is deliberately an operator action rather than automation. Metronome does
emit a resolved webhook on `IN_ALARM → OK`, but only for the
`low_remaining_contract_credit_and_commit_balance` variant and only once
Metronome support enables it, so it is not a dependency worth taking for an
edge case. If dashboard grants become routine, that webhook is the follow-up.

## Migration

None. Existing callers omit `force` and keep the old behaviour.
