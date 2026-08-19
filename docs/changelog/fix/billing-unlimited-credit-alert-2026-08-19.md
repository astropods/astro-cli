# Stop the credit alert from suspending an unlimited account

## Summary

An internal account was suspended for money it does not owe. The unlimited plan
rates every metered product at a zero multiplier, so the account owes nothing,
but it also holds no credit, and Metronome reads an empty balance as an
exhausted one.

## Design

The unlimited plan assumed a zero total removed every suspend condition: no
credit balance to exhaust and no invoice to fail, so the gate needed no internal
case. One condition survives. Metronome's `Low Remaining Contract Credit Balance
Reached` alert has a zero threshold, and a contract provisioned from the
unlimited package carries no credit at all. The alert fires on its first
evaluation, sends
`alerts.low_remaining_contract_credit_balance_reached`, and stays in alarm with
nothing to resolve it. That maps to `SignalCreditsExhausted`, and `computeStatus`
suspends on an exhausted balance with no card on file. Internal accounts have no
card, because the plan is the reason they do not need one.

Measured in preview, where the alert suspended 44 accounts.

**The signal is dropped, not the gate.** `MetronomeWebhookWorker` now discards
`SignalCreditsExhausted` for an account on the unlimited plan and leaves
`computeStatus` alone, so every other suspend reason keeps one code path. Only
this one signal is wrong for an unlimited account: it owes nothing, so it has no
invoice to fail and no spend to cross its own threshold.

**The plan is resolved, not stored.** The worker asks the same question
provisioning asked, the creator's verified address against
`BILLING_UNLIMITED_EMAIL_DOMAINS`. A stored verdict would be one more copy to
keep in step, and a stale copy gates an internal account for money. The cost is
one extra indexed read on an alert that fires once per account.

An empty domain list resolves to false for every account, which leaves an
installation with no unlimited plan behaving exactly as before.

**The billing page names the plan.** An account's plan was only visible by
reading its Metronome contract, so a provisioning mistake looked exactly like a
working account. The billing settings page now states which of the three plans
the account is on, for orgs and personal accounts alike.

The value is the live contract's package, not the plan provisioning would choose
today. Those two disagree in exactly the case worth seeing: an account whose plan
changed after it was provisioned. An account with no covering contract reads
"Not set up" rather than falling back to a plan name, because a provisioning gap
must not look like a working account.

The package id is read from the raw response body. Metronome's API returns
`package_id` on a contract and the Go SDK's `ContractV2` does not model it.

**The architecture doc described a system we no longer run.** It stated that
billing gating was a no-op and that the webhook handlers were log-only, and it
had no account of provisioning at all. Both claims were false, which is worse
than a gap: a reader trusted it and concluded nothing gates.

`docs/03-architecture/billing-data-flow.md` now describes gating as it works,
covers both providers' webhooks in one flow, and gains a provisioning flow with
the three plans. It also records the handful of behaviours that are easy to
misread from the code: the spend warning and limit sharing one alert type, the
id-less event that skips dedupe, the unknown customer that acks instead of
retrying, and the detached-card event that is provisional until Stripe is
re-read.

**Billing now has three docs, for three different readers.**
`billing-overview.md` is new and answers one question in five minutes.
`billing-architecture.md` moves in from a personal notes repo, where it was
useful to one person; it covers the server, the client, the CLI, the operator
tooling, and the three external systems. `billing-data-flow.md` stays the
function-by-function view. Each links to the other two.

The imported document predated the unlimited plan, so it gains the three plans,
the credit-alert exemption, and the fact that provisioning cannot change an
existing plan.

One claim in the imported document was wrong and is corrected rather than
carried over: it said the per-event UUID let Metronome dedupe a retry. A fresh
UUID matches nothing, so a compute span sent twice is charged twice. The document
now says that, and names the two paths that reach it.

## Migration

None for production, which runs `BILLING_PROVIDER=noop` and holds no contracts.

Preview accounts already latched by the alert keep `credits_exhausted` until a
signal clears it. Clearing `billing_provisioned_at` re-runs provisioning, whose
tail applies `SignalCreditsGranted` and reconciles workloads; the contract itself
is untouched, because `ProvisionCustomer` returns early when one already covers.
