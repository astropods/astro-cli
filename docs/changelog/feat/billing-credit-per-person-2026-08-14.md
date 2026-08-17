# Grant signup credit once per person, not once per account

## Summary

A user could mint free credit indefinitely by creating organizations.

`CreateAccount` caps personal accounts at one per user. It applies no equivalent
cap to organizations, and no quota counts accounts: every quota in
`internal/quota` is scoped to one account. Each account created enqueues
`billing.provision`, which creates a provider customer and a contract carrying
the package, and the package grants the signup credit. The contract's uniqueness
key is the account id, so a new account is always a new grant. There is no rate
limiting on `POST /api/v1/accounts`.

Measured on preview: the grant is $10.00 with a one-year access schedule, all 56
accounts are provisioned, 29 of them organizations, and one user belongs to 8.

The loop closes cleanly for an abuser. `credits_exhausted` gates only when no
card is on file, so an organization that spends its $10 suspends and the same
user creates another.

Neither billing platform prevents this. Metronome's fraud tooling is spend
threshold billing, which caps exposure to uncollected revenue per customer;
farming grants is not uncollected revenue, and per customer is the wrong axis.
Stripe Radar scores charges, and no charge occurs. Only we know that those
organizations share a person.

## Design

**The claim is taken against the person.** A new `billing_credit_grants` table
holds one row per user, recording which account took their grant. Provisioning
claims it before asking the provider for a contract, and the answer selects the
plan.

The row outlives the account that claimed it. Deleting an account must not
restore the claim, or delete-and-recreate becomes the same farm with an extra
step.

**The claim is idempotent for the account that holds it.** `ON CONFLICT DO
UPDATE ... RETURNING account_id` returns the holder either way, so a job that
claimed the credit and then failed before creating the contract still reads true
on retry. Without that, one transient failure would silently downgrade an
account to no credit.

**An unresolvable creator gets no credit.** A grant that cannot be attributed to
a person is exactly what this guards against, so the safe answer is to withhold
it. The account is still provisioned, because an account with no contract has its
usage rated by nothing at all, which is worse than a missing $10.

**Two packages, not a bare rate card.** Contract creation is mutually exclusive
between `package_id` and the rest of the contract fields, and it accepts no
`usage_statement_schedule`. Building the credit-free contract from the rate card
directly would therefore bill on a different period from every credit account.
A second package keeps the terms identical and differs only in the grant, which
is what Metronome's package abstraction is for.

**A missing credit-free package refuses.** Falling back to the credit package
would restore the grant silently, which is the behaviour being removed. The
provision job fails, the account stays unstamped, and the hourly sweep re-runs it
once the configuration lands.

## Migration

**Create the credit-free package in each environment before deploying.** It must
carry the same rate card and statement schedule as the existing one and no
credits:

```
POST /v1/packages/create
{
  "name": "Standard Package (no signup credit)",
  "rate_card_id": "<same as METRONOME_PACKAGE_ID's>",
  "usage_statement_schedule": {"day": "FIRST_OF_MONTH", "frequency": "MONTHLY"},
  "uniqueness_key": "astro:standard-no-signup-credit"
}
```

Set `METRONOME_PACKAGE_ID_NO_CREDIT` to the returned id. Preview is done and
wired. Production runs `BILLING_PROVIDER=noop` and provisions nothing, so it
needs both the package and the variable before it switches to Metronome.

**No backfill is needed.** Production runs `BILLING_PROVIDER=noop`, so it holds
no provider customers, no contracts, and no grants. Its ledger starts empty
because nothing has ever been granted there, which is the state this code
expects.

Preview is the only environment that has granted anything, and its ledger starts
empty against accounts that already hold credit. The first new account each
preview user creates will therefore take a second grant. That is test data and
not worth reconciling.

Nothing re-provisions an existing account in either environment:
`ProvisionCustomer` returns early when a covering contract exists.
