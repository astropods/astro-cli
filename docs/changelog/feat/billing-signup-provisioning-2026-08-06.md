# Signup billing provisioning

## Summary

Hosted billing could meter usage but never set an account up to be charged for
it: nothing put a customer on a package, and nothing granted the free credit a
new account starts with. Packaging was treated as out-of-band dashboard work, so
a new signup arrived in Metronome as a bare customer record with no contract and
no balance.

This adds the provisioning step, so a new account lands on the package with its
signup credit already granted.

## Design

**A separate seam.** Provisioning is a new `billing.Provisioner` interface rather
than a method on `BillingProvider`, discovered by interface assertion the same way
`LinkStripeCustomer` already is. That keeps the core provider contract
metering-only and lets the OSS noop backend implement nothing at all. Only the
Metronome backend provisions, and only when a package is configured.

**One contract plus a credit grant, not two tiers.** Every account gets the same
contract on the same package, plus a credit grant. Free and paid are not
separate packaging; the difference is only what happens when the balance runs
out. This is what keeps the free tier from needing a second package and a
contract transition when a card is added.

Contracts are created with `package_id`, which Metronome treats as mutually
exclusive with the rest of the contract fields: only `customer_id`,
`starting_at`, `package_id`, `uniqueness_key`, `transition`, and `custom_fields`
are accepted alongside it.

The signup credit is therefore a standalone credit grant rather than a
contract-inline credit. That restriction rules out inline credits on a package
contract, and a standalone grant is what the existing `Balances` read path
lists, so it is also the only form that surfaces on the billing page.

**Idempotency is provider-side.** Both the contract and the grant carry a
uniqueness key derived from the account ID, so Metronome rejects a repeat with
409 and the worker treats that as already-provisioned. Re-running is safe, which
is what lets both the signup path and the backfill sweep enqueue freely.

That only covers contracts this code created. One made by hand in the dashboard
has no uniqueness key to collide with, so provisioning would stack a second
contract on the customer. It now lists contracts covering the current date and
branches on what it finds: ours means skip, none means create, and anyone
else's is an error rather than a silent skip, because leaving the account
billing on someone else's rates is worse than refusing to provision it. The
credit grant still relies on its uniqueness key, so an account with a hand-made
contract and no credit gets the grant it is missing.

A contract counts as ours by package *or* by the uniqueness key we created it
with. The key is what makes the guard independent of whether the list response
populates `package_id`: matching on the package alone, a contract we made would
read as foreign on any re-check — a retry after the credit grant failed, say —
and that account's provisioning would be cancelled permanently. The list is not
paginated (no cursor on the response, no limit on the params, and the SDK emits
no `ListAutoPaging` for it), so one call sees every covering contract.

River dedupe needs the same care. `UniqueOpts` defaults to a state set that
includes `completed`, so a finished job would block the hourly re-enqueue until
the cleaner removed it — which would strand exactly the accounts the skip branch
below is designed to retry. The job now names its states explicitly, as
`deploy.go` already does.

**Defaults are off, and partial config fails at startup.** `METRONOME_SIGNUP_CREDIT`
and `METRONOME_CREDIT_EXPIRY_DAYS` both default to `0`: a contract is created and
no credit is granted. Granting requires all three of the package, the credit type
and an expiry, and `validateBilling` rejects any subset. That matters because a
grant rejected by Metronome returns a 400 rather than a 409, so it is not absorbed
as already-provisioned — the account would never be stamped and the sweep would
retry it forever. Note there is no never-expires option: both of Metronome's
credit APIs require an end date, so an expiry of `0` would grant credit that is
already dead.

**An unconfigured provider is not a provisioned account.** `ProvisionCustomer`
returns whether it did anything, and the worker stamps `billing_provisioned_at`
only when it did. Without that distinction, deploying before the package id is
set would mark every account done while creating nothing, and the sweep's
`billing_provisioned_at IS NULL` filter would then never match them again. The
cost is that the sweep re-checks unprovisioned accounts hourly while the config
is absent, which is the right trade against a silent permanent skip.

**Only a provisioning backend enqueues.** The noop provider is a real non-nil
value, so a `billingProvider != nil` check at signup would insert a
`billing.provision` job on every OSS account creation, where no worker is
registered for the kind. The handler asserts the `Provisioner` seam instead,
which only Metronome satisfies, matching how the periodic sweep already gates
on the backend.

A customer covered by someone else's contract is the one failure retrying
cannot fix, so it cancels rather than burning a backoff schedule every hour.
Cancelled sits outside the job's unique states, so the sweep still re-checks
the account once per tick and the log stays one clear line per account.

**Off the request path.** Signup used to create the billing customer inline,
blocking account creation on a Metronome call whose failure was logged and
dropped. Customer creation now happens in the same River job as provisioning, so
signup only enqueues. A new `billing_provisioned_at` stamp on accounts drives an
hourly sweep that backfills anything the enqueue missed, including the accounts
that predate this. Preview currently has 56 accounts and 6 billing customers, so
the sweep has real work to do on first run.

## Migration

Provisioning is inert until a package is configured, so deploying this changes
nothing on its own. The `accounts` table gains a nullable
`billing_provisioned_at`; Atlas applies it from the declarative schema.

Enabling it needs Metronome-side setup first, because the package, rate card,
pricing unit, and billable metrics are not provisioned from this repo:

1. **Billable metrics** over the two event types already being ingested:
   `deployment_compute_usage` summing `cu_hours`, and `ai_gateway_llm_usage`
   summing `cost_usd`. Gateway usage is billed as a markup on the dollar cost
   Bifrost already computes per request, not on raw token counts, so one metric
   and one rate cover every model without per-model rates to maintain.

   Note `cost_usd` is only written when Bifrost supplies it, so a model missing
   from Bifrost's catalog produces an event with no cost property and bills as
   zero. Token counts ride along on the same event if a fallback is needed.
2. **Pricing unit**: the credit type customers are billed in. If that is a
   custom unit (AstroAI Credits) rather than USD, `METRONOME_SIGNUP_CREDIT` is a
   count of those credits, not cents.
3. **Rate card** carrying prices for those metrics, and a **package** built on
   that rate card. The package ID is what the server attaches customers to.

Granting a signup credit needs all four of `METRONOME_PACKAGE_ID`,
`METRONOME_CREDIT_TYPE_ID`, `METRONOME_SIGNUP_CREDIT` and
`METRONOME_CREDIT_EXPIRY_DAYS`. None of them have a usable default:
`METRONOME_SIGNUP_CREDIT` and `METRONOME_CREDIT_EXPIRY_DAYS` are both `0`, so
setting only the package and credit type creates a contract and grants nothing.
`validateBilling` rejects a partial combination at startup rather than letting
it run, since a rejected grant returns 400 rather than 409 and would be retried
forever. There is no never-expires option: both of Metronome's credit APIs
require an end date, so an expiry of `0` would grant already-dead credit.

Customers already covered by a contract on the configured package need nothing
done: provisioning recognises it as ours, skips contract creation, and grants
the credit they are missing. Preview's two manually-assigned accounts are on the
configured package, so they follow the standard flow.

Only a contract on a *different* package, or one carrying no package at all,
blocks provisioning. Stacking a second contract risks double-billing and
skipping would leave the account on the wrong rates, so the job cancels and
waits for someone to archive the stray contract in Metronome.

On the next deploy, new signups provision inline and the hourly sweep backfills
existing accounts. Verify by checking that a customer in Metronome has both a
contract on the package and a credit grant.

Enable the package and the credit together. A deployment that runs with a
package and no credit stamps its accounts as provisioned on the contract alone,
which is correct for a no-free-tier deployment but means turning the credit on
later will not reach them, since the sweep only looks at unstamped accounts. To
grant a credit to already-provisioned accounts, clear the stamp
(`UPDATE accounts SET billing_provisioned_at = NULL WHERE ...`) and let the
sweep re-run: the contract is recognised as already ours and skipped, and the
grant's uniqueness key means only accounts actually missing one receive it.

No public documentation yet. The signup credit is user-visible once enabled, but
the account-level story it belongs to is pay-as-you-go, so the `docs-public`
entry lands with that rather than here.
