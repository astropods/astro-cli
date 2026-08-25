# Close the spend-then-delete gaps

## Summary

An account could run up an AI gateway bill it could not pay, then delete itself
and take the record with it. Three controls were missing, and each one is a
separate step in the same sequence.

The gateway ceiling did not reflect what an account could be charged. Every
account carried a flat $20 monthly gateway budget, while a card-less account
holds a $10 signup credit. That gap is deliberate exposure per account, and the
gateway budget is a soft cap on top of it: it authorizes a request before the
provider reports the cost, so concurrent traffic overshoots.

Suspension did not reach the gateway. The suspend worker scaled deployments to
zero and stopped there. Developer keys and the eval judge key answer from
outside the cluster, so a suspended account kept spending, and the dev-key route
was not behind the billing gate at all, so it could mint a fresh one.

Deleting an account destroyed the receivable. The delete handler archives the
billing customer first, on purpose, so charging stops immediately. The provider
archives every contract as of that date and voids the invoices with them, and
nothing checked whether the account owed anything first.

## Design

**The gateway ceiling follows the card.** Two ceilings replace the single
constant: the card-less one matches the signup credit, and the wider one applies
once a card is on file, where the account's own spend limit becomes the control.
A new `billing.gateway_budget` job re-derives the ceiling from
`account_billing_status.has_payment_method` and writes it with
`PUT /api/governance/customers/{id}`. Naming the window the customer already has
rewrites that budget in place, so usage accrued so far carries over and lowering
a ceiling stops an account at once rather than granting it a fresh window.

The job is enqueued wherever the gating status is reconciled, and from the card
add and remove path, rather than on a transition. Every run re-applies the
ceiling, so an account that missed one signal converges on the next.

**Suspension revokes the keys that outlive the workloads.** The suspend worker
now revokes the account's developer keys and judge key upstream after stopping
the deployments. Both are re-minted on demand once the account is back in good
standing, so resume needs no counterpart. Per-deployment keys stay: their
workloads are already stopped, and the key value lives in a tenant Secret that
resume re-applies rather than re-mints. The revoke is best-effort, so a gateway
outage cannot undo a suspension that already stopped the workloads. The dev-key
route is now behind the entitlement gate and answers 402 to a suspended account.

**A delete with an outstanding balance is refused.** Before archiving, the
handler asks the provider what the account owes and answers 409 when it owes
anything. Two facts count, because they describe different periods: an open
draft invoice with a nonzero total is usage run up in the current period, and
dunning is a finalized invoice that failed to collect, which the draft does not
carry. The draft total is net of credit drawdown, so an account still inside its
grant deletes freely. A provider that reports neither is not treated as owing,
and a failed read is an error rather than a silent pass.

**Removing the card is the same escape.** Deleting the account and removing the
payment method both end the billing relationship, and only one of them was
guarded. Removing the card stopped future spend, because the card-removed signal
drops the account to the free-tier floor, but the accrued draft was never
charged. Mid-period spend lives in a draft invoice, and a draft cannot be
collected: the provider refuses a pay call on anything that is not finalized.

So removal now runs the same balance check and answers 409. A failed read keeps
the card attached, because that is the recoverable outcome. Changing cards never
needed removal in the first place: saving a new card detaches the old one in the
same call, so the refusal points there instead of leaving the account stuck.

**The gateway ceiling tracks the account's own spend limit.** A customer sets
one limit for total account spend, compute units and gateway together, so a
separate gateway number either caps them below the figure they chose ($500 set,
$20 enforced) or leaves the gateway free to exhaust it alone. The budget worker
now reads the limit and applies it, falling back to the card-derived default only
for an account that has set none, so the gateway is never uncapped.

**The re-derive is unconditional, and runs before the error branch.** Every
outcome of a threshold write can have moved the limit. A success sets it, a null
clears it, and a failure can leave it unset, because replacing a threshold
archives the old alert before creating the replacement, so a create that fails
removes the limit and reports an error. That last case is the one that most needs
re-deriving: the account now has no limit while the gateway still enforces the
old number, which is the over-permissive direction. Keying the enqueue on success,
or on the limit landing in `applied`, would skip exactly it.

**A post-write reconcile outlives its request.** These run after the provider
write has landed, so tying them to `c.Request.Context()` means a client that
hangs up cancels the insert and leaves our side stale with nothing queued to
correct it. All four sites now detach with `context.WithoutCancel` under a
15-second bound: the two latch lifts, the gateway re-derive on a threshold write,
and the status write plus re-derive on a card change.

Matching the limit does mean the gateway could spend the whole of it on its own.
The provider still suspends on the combined total; this is the control that stops
an uncollectible account inside the minutes that takes, and it is enforced
synchronously at the gateway rather than on an ingest round-trip.

**A sweep, not a notification, is what makes the ceiling correct.** The three
paragraphs above are all the same defect: a real-time control derived from a
mutable value, with a gap in what happens when that value moves. Each was fixed
by adding an enqueue to the path that moved it, which only ever covers the paths
already thought of. Nothing structurally forces a fourth writer to notify, and a
notification that never arrives is wrong in both directions: too high and the
spend is uncollectible, too low and a paying customer is blocked under the limit
they chose.

So a periodic sweep now re-derives the ceiling for every account holding a
gateway customer, and the enqueues become a latency optimization rather than the
correctness guarantee. Both paths call the same `applyBudget`, so they cannot
derive different numbers. It runs every 15 minutes, shorter than the other
billing sweeps because this ceiling is enforced in real time, and it also
corrects a budget that drifted inside the gateway itself.

Each tick takes a bounded slice ordered by staleness, oldest applied first and
never-applied before those. The ordering is the whole design. The set of accounts
holding a gateway customer does not shrink as they are processed, unlike the
dunning worklist, so a slice ordered any other way means the accounts past the
bound are swept never rather than late: every tick would restart from the same
end. Ordered by staleness, an account left out of one tick sorts earlier in the
next, and a tick cut short loses only work that leads the following one.

Removing the bound instead does not avoid this. River cancels a job at its
timeout, so an unbounded tick over a large enough table is bounded anyway, at a
point we neither chose nor can see. The bound is explicit for that reason, the
worker's own timeout is set above what a full slice needs, and a tick that fills
its slice logs that it did, because a full slice means the staleness window is
wider than the interval suggests.

The stamp is recorded after a failed apply as well as a successful one. Skipping
it on failure would leave a permanently broken account at the front of the
ordering on every tick, holding a slot nothing else can use. The failure is
logged instead, and that account is retried on a later tick.

The stamp also runs on a context detached from the tick's deadline, under a
five-second bound of its own. Sharing the deadline would defeat the stamp in the
one case it matters most: an account slow enough to exhaust the tick fails its
apply and then fails its stamp, so it stays the stalest account, leads the next
tick, and starves everything behind it again. That is the same failure as an
unstamped exception, arriving through the clock rather than through an error.

**A spend limit stops where self-serve stops.** The limit was bounded only by a
typo guard, so a customer could put themselves on the hook for $10 million and
the gateway would enforce it. A limit is collectible only up to what the card
behind it settles, which makes an unbounded self-serve number our exposure rather
than the customer's. It is now capped at $1,000 per month, matching where both
OpenAI and Anthropic end self-serve and require an increase request, and the
rejection names the ceiling.

The rejection points at an enterprise conversation, not at the self-serve quota
increase route. That route validates against the resource quota keys and has no
concept of a spend limit, so a customer sent there gets an invalid-key error, and
approving one could not raise the cap anyway: it is one number for everybody.
Above $1,000 the answer is a plan, not a form.

Bounding it at the point the customer sets it, rather than clamping the derived
ceiling, is what keeps the two numbers honest: a silent clamp would show a limit
on the settings page that the gateway does not enforce. The worker still clamps
as well, because a limit can reach the provider without passing the handler when
an admin or a backfill writes one directly.

Usage thresholds keep the typo guard. They carry a different unit per metric, so
a dollar ceiling would wrongly narrow a CU-hour budget.

## Migration

Spend limits above $1,000 per month are now rejected. The highest limit set
today is $50 and provisioning seeds $20, so nothing existing breaks. An account
that needs more is an enterprise conversation rather than a self-serve setting.

Apply `sql/astro-server/schema.sql` before deploying astro-server. It adds
`accounts.gateway_budget_swept_at` and its partial index, which order the sweep.
Additive, no backfill: a NULL sorts first, so every existing account is treated
as never swept and is picked up on the first ticks.

Nothing else is required. Every account's gateway ceiling converges within a few
sweeps of deploy rather than waiting for its next billing signal, so a card-less
account created before this change no longer keeps its wider ceiling
indefinitely.
