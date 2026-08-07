# Make BILLING_GATE_ENFORCE the whole switch

## Summary

`BILLING_GATE_ENFORCE` reads like the on/off switch for billing enforcement, and
its own comment says so: "false = observe/log, true = enforce". It was wired
into exactly one place, the entitlement middleware that returns a 402. Every
other consequence of a suspended status ignored it.

So on `main` today, a Stripe `invoice.marked_uncollectible` scales an account's
deployments to zero with the flag off, and once credit-exhaustion gating lands
a Metronome balance alert does the same. Reaching for the flag as a kill switch
during rollout would not have stopped either.

## Design

**Bookkeeping always runs; acting is gated.** The status machine still consumes
every signal, recomputes, and persists status and reason regardless of the flag.
That is the point of observe mode: deploy it off, watch the transitions in the
logs and in `account_billing_status`, and confirm the machine is right before
anything acts on it.

What the flag now controls is the three user-visible consequences: the 402, the
workload suspension, and the owner notifications.

**Suspension is gated at the queue, not at the callers.** Five places reconcile
workloads — both webhook workers, the dunning sweep, the provision worker, and
the card handlers — and threading a boolean through all of them invites the one
that gets missed. `InsertBillingSuspend` is the only route to the job, so gating
there is total by construction and cannot be bypassed by a caller added later.
In observe mode it logs the suspension it declined, keyed by account.

Notifications get an explicit `EmitBillingNotify` rather than filtering inside
`EmitNotify`, so build failures and the rest are unaffected by a billing flag.

**Resume is not gated.** Suspend is enforcement; resume is remediation, and
coupling them would make the kill switch one-way: turn enforcement off after a
real suspension and the account stays at zero replicas, still stopped, even once
it fixes its card and recomputes to active. Leaving resume open costs nothing,
because `BillingResumeWorker` only restores deployments in `StatusSuspended` and
nothing but billing puts them there — with enforcement off there is nothing for
it to find. Flipping the flag off does not itself sweep existing suspensions
back up; it means the next signal for an account can.

**The banner reports what happened, not what is configured.** In observe mode
the endpoint would otherwise report `suspended` while nothing is suspended, and
the client would tell users their agents had stopped when they were still
running. But `enforced` alone is the wrong test in the other direction: a
suspension is durable, so turning enforcement off after one leaves the account
at zero replicas with the banner silent — agents genuinely down and the UI
saying nothing.

`GET /billing/status` therefore returns two facts. `enforced` is whether the
status is acted on; `workloads_suspended` is whether billing has already stopped
this account, read from the deployments themselves. The banner renders when
either holds, so observe mode stays quiet and a real suspension stays visible
however the flag moves afterwards. The endpoint is polled on every page, so the
second read is skipped for an active account, which cannot raise a banner
either way, and it lives on `deploymentstore` rather than having the billing
store reach into a table it does not own.

**Recovery notices are suppressed in observe mode.** Resume is ungated but its
notification is not, so an account restored after enforcement was turned off
gets its agents back without an email saying so. The banner does clear, since
`workloads_suspended` goes false once resume runs, but the loop opened by the
suspension email is closed only in the UI. Letting recovery notices through
unconditionally would be worse: `SignalRecovery` comes from `invoice.paid`,
which fires for accounts that were never suspended, so observe mode would start
emailing people about a problem they never had.

## Migration

This turns gating on in **preview**: `BILLING_GATE_ENFORCE=true` in
`config/astro-server/preview.env`. Prod is untouched and stays `false`, on top
of `BILLING_PROVIDER=noop`, which already makes the flag inert there.

The flag takes effect when the ConfigMap is applied, so merging alone changes
nothing until the Sync Secrets & Config workflow runs against preview and the
pods roll. After that, a preview account that spends its free credit without a
card is scaled to zero and emailed. Revert by setting the value back to `false`
and re-running the sync.

Two prerequisites, or gating is silently inert rather than wrong: the Metronome
**Contract credit balance** alert at threshold `0` must exist and be subscribed
on the webhook endpoint, and the `billing.credits_exhausted` workflow must be
authored in Novu. Without the alert nothing suspends; without the workflow
accounts are suspended without being told why.
