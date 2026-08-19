# Put internal accounts on an unlimited plan

## Summary

An account owned by a company address is measured like any other and never owes
anything. Employee usage stayed on the standard plan, so it drew the same $10
signup credit, exhausted it, and then hit the same gates as a customer.

## Design

**A third package, not an exemption in the gate.** The plan carries the same rate
card and statement schedule as the other two, with every metered product
overridden to a zero multiplier. Usage meters, rates, and appears on the period
statement exactly as before; the total is always zero. Nothing owed means no
suspend condition can be reached: there is no credit balance to exhaust and no
invoice to fail, so `computeStatus` needs no internal case and the gate keeps one
code path for everyone. Putting the guarantee in the rating means it holds
whether or not the gate logic is correct, where an exemption inside
`computeStatus` would have to be kept in step with every future suspend reason.

**The plan is one value, decided once.** `ProvisionCustomer` took a `withCredit`
bool, which cannot express a third option. It now takes a `billing.Plan` of
`PlanCredit`, `PlanNoCredit`, or `PlanUnlimited`. A missing package for the
chosen plan is a configuration error rather than a reason to fall back, matching
what the no-credit plan already did, because falling back would silently bill an
internal account. The hourly sweep re-runs the job once the configuration lands.

**An internal owner never reaches the credit ledger,** so an employee account
does not spend the person's one grant on a plan that has no use for it.

**Matching is exact, on a verified creator address.** The domain after the last
`@` must equal a configured entry, so neither a subdomain nor a lookalike like
`evil-postman.com` matches.

The address comes from `GetCreatorVerifiedEmail` rather than `GetOwnerEmail`,
because the plan is an entitlement and the existing lookup is built for a label.
It requires a verified address, so a domain anyone can assert does not earn free
usage, and it pins to the creator instead of joining across members, so a later
member's domain cannot decide the plan when the creator has no mirrored address.
A creator with no verified address takes the standard plan, which keeps the
account rated rather than stalling the job.

**The misconfiguration is refused at boot.** `BILLING_UNLIMITED_EMAIL_DOMAINS`
defaults to a value and the package cannot, because a package exists only inside
one Metronome environment. An environment with provisioning on, a domain list,
and no unlimited package would otherwise resolve the plan per account and fail
that account's job every time. `Validate` rejects the combination instead, where
the environment is being configured and someone is reading.

## Migration

Set `METRONOME_PACKAGE_ID_UNLIMITED` for any environment running
`BILLING_PROVIDER=metronome` with provisioning on, or the server refuses to
start. Preview points at the sandbox package; production needs the package
created in its own Metronome environment first, because packages do not cross
environments.

`BILLING_UNLIMITED_EMAIL_DOMAINS` is comma-separated and defaults to
`postman.com`. Set it to an empty value to turn the plan off entirely, which
also drops the package requirement. Existing accounts keep the plan they were provisioned on; moving
one means archiving its contract and provisioning it again.

**The claim outlives the account.** `billing_credit_grants` is keyed on the
person and carries no foreign key to `accounts`, so neither a soft delete nor
the retention purge returns the claim. An integration test hard-deletes the
holding account and then claims again, which keeps deleting and signing up from
earning a second grant.
