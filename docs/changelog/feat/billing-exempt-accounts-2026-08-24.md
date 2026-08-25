# Never suspend an exempt account

## Summary

Two organizations must never have their deployments stopped by billing. Today
that is expressed as a third Metronome package that rates every product at zero,
chosen at provisioning time from the creator's email domain.

That package protects against charges, not against suspension. `computeStatus`
suspends on `not_provisioned` first, which is the flag an expired contract, a
misconfigured package id, or an unreachable provider all raise. A zero rate does
nothing about any of them, so the guarantee holds only while Metronome is
healthy and configured correctly.

It is also decided once, from the wrong input. The plan comes from the account
creator's email domain, so an organization created by anyone outside those
domains silently gets the wrong plan for the rest of its life.

## Design

**One flag, checked before every reason.** `BILLING_EXEMPT_ACCOUNTS` holds
account ids. `computeStatus` returns active for them ahead of all six suspension
reasons, so no provider state can reach the decision:

```go
if s.exempt {
    return StatusActive, ""
}
```

**Exempt from suspension only.** Usage still meters, the contract still rates,
invoices still total, and every flag still records on the account. An exempt
account is auditable exactly like any other one. This changes what stops a
deployment, not what anything costs or what gets tracked.

**Enforced at every read, not just the write.** `Recompute` decides what to
store, but three readers see the stored row directly. The gate reads `Record`,
because a block has to name the fix and the pay link lives on that row, and the
billing endpoints read it too. `Get` and `Record` both honour the exemption, so a
row written before the account was exempt cannot keep returning a 402.

`readRecord` stays raw on purpose. `Recompute` reads through it, and if it saw
the corrected status it would match what it was about to write and skip the
correction, leaving the stored row suspended forever.

**The worker gets the same set.** The dunning sweep suspends on its own timer, so
it would otherwise suspend an exempt account behind the gate's back.

**An account suspended before it became exempt is resumed, not just unblocked.**
The read override alone is not enough. A suspension stops the workloads and marks
the deployment rows, and `ListInDunning` only scans `past_due`, so an account
already at `suspended` is never re-evaluated. Its reads would report active while
its agents stayed down, and the override would have removed the 402 that was the
only sign anything was wrong. The sweep therefore recomputes every exempt account
and enqueues a resume, which touches suspended deployments and nothing else.
Running it each sweep rather than once at startup makes it self-healing whichever
order the suspension and the exemption arrive in.

**Ids, not slugs.** The id is what the gate holds, so matching on it needs no
join, and the record query cannot grow one: `Recompute` reads `FOR UPDATE`, which
Postgres refuses on the nullable side of an outer join. A rename also cannot
silently drop the protection.

**Fixed at construction.** `WithExemptAccounts` is called once while the store is
built, so the set is read without a lock.

**Removing an exemption re-suspends a delinquent account.** The exemption
persists `active`, which is what lets the workloads resume, but it also means the
stored status no longer says the account owes anything. The sweep's work set was
`past_due` only, so an account that went delinquent while exempt would have sat
outside every work set and run unpaid indefinitely once the exemption came off.

`ListForRecompute` replaces `ListInDunning`: past_due, plus rows stored active
whose own flags say the machine would not have produced active. The predicate
mirrors `computeStatus` rather than listing every flag, so a carded account with
spent credits stays out of the work set, and suspended accounts stay out too,
which is what keeps this from re-evaluating every stopped account on every tick.

Currently exempt accounts are excluded from it. They match the drift half by
definition, and a recompute that changes nothing leaves `updated_at` alone, so
they would sort to the front of a `LIMIT 500` set on every tick and hold slots
real work needs. `reconcileExempt` covers them instead.

Two details in that predicate are load-bearing. The exclusion is parenthesised
around the whole status test, because bound to the drift branch alone it would
still sweep an exempt past_due row. And the exempt set is one array parameter
rather than generated placeholders, which keeps the query static, but the array
must never be nil: `<> ALL(NULL)` is NULL, so a nil would match no rows and
silently empty the work set.

## Migration

Nothing to do, and nothing takes effect yet. Every worker here sits behind the
Metronome backend, so none of it runs where the provider is noop, and the
environment that does run it has no exempt ids set. The two configurations are
mirror images: production carries the ids and not the workers, preview runs the
workers and carries no ids. The protection is dormant until those meet.

`BILLING_EXEMPT_ACCOUNTS` is set in the production config to the astropods and
postman organization ids, ahead of the Metronome cutover rather than with it. The
gate reads the set at construction, so an account already suspended when its id
arrives stays stopped until a sweep recomputes it: arriving a deploy early costs
nothing, arriving a deploy late costs a suspension.

Exercising the exemption before that cutover needs accounts in preview to point
it at, and the two organizations named above exist only in production.

The exemption also decides the AI gateway ceiling, which is a second way to stop
an account and one that suspension status does not reach. Every gateway customer
carries a monthly budget from the moment it is created, and provisioning seeds
every account a spend limit, so an exempt organization would otherwise be stopped
at $20 of model spend by a control that has nothing to do with dunning. Raising
the budget by hand in the gateway would not hold either, because the ceiling is
re-derived on a schedule and would be written straight back down.

An exempt account gets the standard $1,000 ceiling as a floor, not an unlimited
one. Provisioning seeds every account a $20 limit and an account without one
falls back to a lower card default, so both paths would otherwise stop an account
billing never suspends.

An unreadable limit writes nothing rather than falling back to the floor. Falling
back would overwrite an operator-raised ceiling with a lower one every time the
provider was unreachable, so the account keeps the number it already has and the
job retries.

Going above the floor is an operator action rather than a larger default. A limit
set above the self-serve bound is honoured unclamped for an exempt account,
because that bound governs what a customer can choose for itself and not what an
operator grants. Nothing self-serve can reach it, so raising an exempt account is
deliberate by construction.

That creation-time budget is worth separating out, because it is the one control
here that does not depend on the billing backend. Every gateway customer gets it
when its first key is minted, wherever the gateway is enabled. Lifting it is the
budget worker's job, and the budget worker only runs under Metronome, so in an
environment with the gateway enabled and billing on noop an exempt account still
carries it.

The two exempt organizations should then move off the unlimited package onto the
normal one, so their usage rates and their invoices total like everyone else's.
Once they have, `METRONOME_PACKAGE_ID_UNLIMITED` and
`BILLING_UNLIMITED_EMAIL_DOMAINS` have no remaining purpose and the package can
be archived. Both are still read by provisioning, so leave them set until that
move happens.
