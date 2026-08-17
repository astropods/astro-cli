# Make a billing notification fail loudly, and let the spend warning send one

## Summary

An account can set two spend controls. The limit suspends it; the warning is a
heads-up that changes nothing. The warning had no way to reach anyone.

Gating and notifying shared one path. `metronomeSignal` returns no signal for the
warning, correctly, because gating on it would suspend an account for crossing
the line it asked to be told about. But `billingAlert` is only reachable through
a signal, so skipping the gate skipped the notification too. An owner who set
"warn me at $80" was told at $80 by nothing: no email, no in-app message, no
banner. The only trace was a `Reached` badge on the billing page.

A second failure hid the first. Novu answers `201` for a workflow that is
switched off, reporting it as `trigger_not_active` in the body. `Trigger`
discarded the body and checked only the status code, so the delivery job
completed and logged success while nothing was sent. That is quieter than a
missing workflow, which at least answers `422`.

## Design

**The warning gets its own event type.** `billing.spend_warning` is separate from
`billing.spend_threshold` rather than a second use of it. The limit's message
states that agents stopped, which is true for the limit and false for the
warning, so one workflow cannot serve both. The worker takes the notify path and
returns before reaching the signal, which keeps "notifies" and "gates" mutually
exclusive by construction:

```go
if isSpendWarning(job.Args.EventType, job.Args.AlertName) {
    return w.notifySpendWarning(ctx, job.Args)
}
```

**A trigger that delivers nothing is now an error.** Several statuses mean the
provider accepted the trigger and sent nothing: the workflow is switched off, it
defines no live steps, the environment has no provider for its channels.
`Trigger` reads the response and fails on any status other than `processed`,
naming it in the error so the log says which one fired.

**Only the configuration statuses cancel.** `trigger_not_active`,
`no_workflow_active_steps_defined`, `no_workflow_steps_defined`, and
`subscriber_id_missing` describe the environment, so a retry repeats them.
Those return `ErrNotDelivered`, which `permanentDeliveryError` treats like a
4xx, and the job cancels at error level.

Every other status retries on a bounded backoff, including Novu's catch-all
`error` and any status the enum grows later. Those cover provider and internal
failures that a second attempt can clear. Cancelling on an outcome we cannot
prove is permanent would discard the notification, and a threshold crossing
fires once, so the owner would never hear about it. Losing a message loudly is
still losing it.

**Notification retries are capped at ten attempts.** River's default of 25 backs
off by attempt^4 seconds and spans weeks. Retrying a status we cannot classify is
the right call, but an environment that answered one consistently would otherwise
churn on every notification it sends, for a fortnight, per message. Ten attempts
covers roughly four hours, which is as long as an outage plausibly lasts and as
long as the notification is still worth delivering.

Reading the response must not turn a successful send into a retry. An empty or
whitespace body reports no status and counts as delivered, which is what the code
did before it read the body at all.

**Both spend messages carry their numbers.** The webhook already sends
`threshold` and `current_spend`; they now reach the payload as `threshold` and
`spent`, formatted as currency because a template cannot divide minor units into
a currency string.

The values travel in a `notifyFacts` struct rather than as two more parameters.
`applyWebhookSignal` is shared with Stripe, which has no spend amounts and only
ever sets the hosted invoice URL, so one struct beats a parameter per signal.

**Provider amounts are decoded as numbers, not integers.** One envelope decodes
every Metronome webhook, including the limit event that suspends an account.
Metered spend accrues fractional cents, and an `int64` field rejects `8034.5`
outright, so a single fractional amount would answer `400` and drop the gating
event for the sake of a number only the message text uses. The amounts decode as
JSON numbers and round to whole minor units.

A threshold of zero renders `$0.00` rather than blank. The provider reports zero
as a real limit, so blanking it would leave the message naming no number.

The warning's emit takes its account lookup and a narrow notifier interface
rather than the concrete queue, following `dunningQueue`. The path that carries
the whole feature is then testable without River: what it emits, an unknown
customer acked rather than retried, a transient lookup error returned, and a
failed emit that must not fail the webhook the gating decision travels on.

## Migration

**Provision the workflow before deploying.** `billing.spend_warning` must exist
and be active in an environment before code that triggers it runs there. The
provider fires a threshold crossing once, and an inactive workflow cancels
rather than retrying, so an owner misses that crossing permanently. It is
authored and active in the preview environment; production is a separate Novu
environment and needs the same.

**Failing loudly applies to every workflow, not just billing.** A workflow that
is inactive in some environment now cancels and logs at error level where it
previously completed in silence. The notify catalog is 16 types, and all 16 are
active in both preview and production.
