# Contract coverage is by existence, not by authorship

## Summary

Provisioning refused to touch any contract that did not carry its own uniqueness
key, treating it as terms it must not disturb. That guard was built for a shape
this system does not have. There is one package: every customer goes on it, gets
$10 of credit, and moves to pay as you go when the credit runs out or a card is
added. A contract created by hand therefore carries the same rates as one
provisioning would create.

The consequences were real. An account onboarded outside the normal path blocked
forever, logged an error every hourly sweep, and never received its signup
credit. The only remedy the rule allowed was to end a working contract, on an
account that may already have finalized invoices, so provisioning could recreate
an identical one with a key attached.

## Design

**One question, one answer.** If a contract is effective now, the customer is
already being billed, so creating another would bill them twice. If none is,
create one. The list still happens because a contract created by hand carries no
uniqueness key, so the 409 that normally makes creation idempotent would not
fire against it.

That removes the whole classifier: the three coverage states, the blocked
sentinel error, the job cancellation it drove, and the rate-card lookup that had
been added to work around the guard being too strict. Two mechanisms that
cancelled each other out are now none.

**Authorship is not reported at all.** The admin view no longer says who created
a contract. With one package the answer changes nothing an operator would do,
and the only signal available for it, the uniqueness key, is not written
consistently: some contracts carry the key this system writes, some carry a bare
account ID from another writer, and contracts made by hand carry none.

**The signup credit is a grant, not a contract credit.** Metronome models the
two separately, and the spend view read the contract-credit endpoint, so every
account reported no credit while holding one. It now reads the grant list,
filtered to grants that have not expired.

**Money is denominated in the credit type's own unit.** That unit is USD
(cents), so a grant of 10 is ten cents, and the raw number rendered a
hundredfold high. The provider now converts before returning, keyed on the
credit type id rather than its display name, so the unit never depends on a
string Metronome is free to reword and no caller rescales money. The preview
signup credit moves from 10 to 1000 to grant the $10 the product offers.

**The strictness that mattered is unchanged.** The check exists to stop a second
contract stacking on a customer already being invoiced, and "any coverage means
create nothing" is a stronger guarantee than the rule it replaces, not a weaker
one. If differentiated pricing ever arrives, this is the point to revisit, and
even then refusing to stack remains correct.

## Migration

Run **Sync Secrets & Config** against preview so the new signup credit applies.

Accounts blocked by a contract created outside provisioning are picked up by the
next hourly sweep. They keep that contract and its invoices, and receive the
signup credit they never got.

Accounts already provisioned are not topped up to the new amount. Their grant
carries uniqueness key `signup-credit:<accountID>`, so the sweep gets a 409 and
skips; raising the amount reaches new accounts only. Preview accounts holding a
ten-cent grant need a manual top-up or nothing at all.
