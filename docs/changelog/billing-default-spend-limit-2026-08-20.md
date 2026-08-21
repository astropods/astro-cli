# Default spend limit on new accounts

## Summary

A new account had no spend ceiling of its own. The billing page offers a spend
limit and a spend warning, but both start unset, so an account only gets a cap
when its owner opens the page and sets one. An agent that loops on the gateway
can spend far past the signup credit before anyone looks.

Provisioning now gives each new account a $20 spend limit for itself.

## Design

The cap is the same control the billing page writes, not a second mechanism. It
is one Metronome customer alert on the account's own customer, so the existing
read shows it in the form, the existing write changes it, and the existing
webhook path suspends the account when it fires. The owner can raise, lower, or
clear it like any limit they set by hand.

`BillingProvisionWorker` seeds it after the account is on the rate card and
before the provisioned stamp lands:

```go
seeded, err := w.accounts.IsBillingProvisioned(acct.ID)
if !seeded {
    if err := w.seedSpendLimit(ctx, acct.ID, customerID, plan); err != nil {
        return err
    }
}
```

Two properties come from that ordering.

The stamp is the once-per-account latch. Provisioning re-runs on an existing
account, because an operator credit grant enqueues the same job, and reseeding
there would silently reimpose a cap the owner had raised or cleared. Asking for
the stamp before writing it means the seed happens on the first run only.

A failed seed blocks the stamp. The stamp is what removes the account from the
hourly sweep, so stamping after a provider error would leave the account
uncapped with nothing to retry it. Returning the error keeps the account in the
sweep instead.

Unlimited accounts are skipped. That plan exists to exempt internal accounts
from billing, and a $20 cap would suspend the accounts it exempts.

The threshold is stored in the provider's cents, so the constant is 2000. This
is separate from the $20 monthly budget the gateway holds on the account's
Bifrost customer. That one caps gateway spend at admission and overshoots under
concurrency; this one covers rated spend across both metered metrics and
suspends the account.

## Migration

None. Existing accounts keep whatever they have, including no limit at all.
