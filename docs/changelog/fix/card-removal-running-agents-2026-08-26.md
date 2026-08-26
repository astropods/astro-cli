# Hold the payment method while an account is still spending

## Summary

An account with a balance removed its card and left the balance uncollectable.
The account had 1.35 cents of draft usage, and the billing page showed it as
`$0.01` while the removal went through.

Two separate holes let it out.

**A balance under a cent read as nothing owed.** The guard compared the raw
dollar figure against a `0.01` floor. At the moment of removal only gateway
usage had landed, 0.9846 cents, which is `$0.009846`. That is below the floor, so
the guard said nothing was owed, while `formatMoney` rounded the same number to
`$0.01` on the page. Stripe settles in whole cents and would have charged one, so
the money was real. The comparison now rounds to cents before testing, which is
the unit both the invoice and the display use. True dust, under half a cent, is
still removable.

**A running agent's spend had not been billed yet.** Compute is metered on
five-minute windows that must close before `MeteringWorker` emits them, so an
agent can run for minutes with nothing in the draft invoice. The balance check
reads that invoice, so it cannot see spend the account is certainly incurring.
The compute half of this account's usage, 0.366 cents, landed after the card was
gone.

## Design

**Removal now requires the account to stop spending first.** A card cannot be
removed while any deployment is pending, provisioning, deploying, active, or
undeploying. Pausing or deleting every agent is the way through, and both leave
the account payable.

The count reads `deployments` rather than `deployment_billing_state`. The latter
is advanced by the same five-minute tick, so a deployment that just went live is
not in it yet, which is the case that has to be caught.

`RunningStatuses` lives beside the status constants and is tested against the
full set, so a status added later cannot default into "not running" unnoticed.

## Notes

The removal dialog still tells the reader their running agents "will stop" as a
consequence of removing the card. That is now a precondition instead. The copy is
unchanged here and needs a follow-up.

## Migration

None. An account with running agents must pause or delete them before removing a
card.
