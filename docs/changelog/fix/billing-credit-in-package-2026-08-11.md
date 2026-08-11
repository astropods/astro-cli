# The signup credit belongs to the package

## Summary

The signup credit was created as a standalone credit grant, one API call after
the contract. Metronome models a grant and a contract credit as different
objects, and almost everything downstream expects the second: the exhaustion
alert watches contract credits, the dashboard lists contract credits, and the
spend read went to the contract-credit endpoint. So no account ever showed a
balance, and the gate that suspends an account for spending its credit without
a card could not fire, because there was never a balance to cross zero.

Nothing was wrong with the model. A Metronome package can carry credits, and
ours was created without any.

## Design

**The package grants the credit, so provisioning does not.** A package credit
is expressed as an offset from contract start rather than an absolute date, so
one package definition serves every customer:

```json
"credits": [{"product_id": "…", "access_schedule": {
  "credit_type_id": "…",
  "schedule_items": [{"amount": 1000,
    "starting_at_offset": {"unit": "DAYS",  "value": 0},
    "duration":           {"unit": "YEARS", "value": 1}}]}}]
```

`ProvisionCustomer` is now a single write: create the contract from the package.
Metronome attaches the credit. The contract's uniqueness key still makes a
repeat create 409, and the existing-contract check still stops a second contract
stacking on a customer already being invoiced.

**Amount, unit, and expiry stop being astro configuration.**
`METRONOME_SIGNUP_CREDIT`, `METRONOME_CREDIT_TYPE_ID`, and
`METRONOME_CREDIT_EXPIRY_DAYS` are deleted along with `validateBilling`, whose
whole job was catching partial combinations of the three. The package is the one
place the offer is defined, which is also the only place that can express it as
a template.

**Spend reads contract credits again.** The grant-list read added while chasing
the empty balance goes back to the contract-credit endpoint with
`include_balance`. The unit conversion and the partial-failure handling are
unchanged.

## Migration

Run **Sync Secrets & Config** against preview. `METRONOME_PACKAGE_ID` points at
the package that carries the credit; the previous package has none, so an
account provisioned against it gets a contract and no balance.

Preview customers were already moved onto the new package, so no backfill is
required there. Production has no Metronome configuration yet; when it gains
one, the package must carry the credit or the exhaustion gate will not engage.
