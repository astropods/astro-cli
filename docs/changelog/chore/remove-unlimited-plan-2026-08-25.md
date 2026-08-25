# Remove the unlimited plan

## Summary

Provisioning put any account whose creator held a verified `@postman.com`
address onto a package that rated every metered product at zero. Usage still
metered, and the statement never totalled anything. The plan existed so
internal accounts could run without a bill, but it made those accounts unable
to exercise the billing system they were meant to test: a spend limit cannot
fire against a statement that is always zero, so caps, alerts, suspension, and
collection were all unreachable on the accounts the team actually uses.

Internal accounts now provision like any other. This removes the plan itself,
not just the domain list, so no environment can turn it back on by setting an
env var.

## Design

**`plan()` resolves from the credit ledger alone.** It read the creator's
verified address first and answered `PlanUnlimited` before consulting the
ledger, so an internal account never spent the person's one signup claim.
With the branch gone the function is the ledger check: a person's first
account takes `PlanCredit`, every later one takes `PlanNoCredit`.

`billing.Plan` loses `PlanUnlimited`, so a plan the provider can no longer
report is also a plan no consumer can branch on. `provisionPackage` and
`planForPackage` lose their unlimited cases, which means the package id is
neither written to a new contract nor recognised on an existing one.

**The webhook no longer exempts anyone from credit exhaustion.** A
credit-exhaustion alert used to resolve the account, read its creator's
address, and return early for an internal one, on the reasoning that an
account owed nothing could not be gated for owing it. That reasoning went
with the zero rating. The handler now refreshes the card fact and applies the
signal for every account, which also drops two database reads from the hot
path of that event.

**Configuration.** `METRONOME_PACKAGE_ID_UNLIMITED` and
`BILLING_UNLIMITED_EMAIL_DOMAINS` are no longer read, and the boot check that
demanded the first whenever the second was non-empty is gone with them.
`getEnvSliceOrOff` existed only to let an environment express "off" for that
domain list and is removed as well.

**Dead code removed with it.** `hasEmailDomain` compared the part after the
last `@` so that neither a subdomain nor a lookalike matched. Nothing decides
an entitlement from an address any more, so it and
`AccountStore.GetCreatorVerifiedEmail`, whose doc comment described exactly
that job, both go. `claimSignupCredit` keys on the creator's user id and is
unaffected.

**Client.** `PlanSummary` and `SpendControls` still branch on
`plan === "unlimited"`. Both files are deleted by the paired Billing and Usage
branch, so this change leaves them alone rather than creating a conflict. The
branch is unreachable either way once the server stops reporting the plan.

## Migration

Preview drops `METRONOME_PACKAGE_ID_UNLIMITED` from its environment. Nothing
else changes: `BILLING_UNLIMITED_EMAIL_DOMAINS` was never set anywhere, so no
environment loses a value it had.

The 44 preview accounts that held an unlimited contract were moved onto the
credit package before this change, so no account is left pointing at a package
the code no longer knows. Each took a fresh signup credit with the new
contract, granted by the package rather than through the claim ledger, so
those accounts spend that credit before any cap applies to them.

The unlimited package itself is archived in Metronome. Provisioning cannot
reference it, and no contract still runs on it.
