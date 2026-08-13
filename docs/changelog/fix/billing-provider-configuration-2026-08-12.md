# Route Metronome invoices to Stripe through a billing provider configuration

## Summary

Adding a card vaulted it with Stripe and told Metronome the Stripe customer ID,
but no invoice ever reached Stripe. Every finalized invoice failed delivery with:

```
No token found for environment type SANDBOX and billing provider STRIPE
```

The cause was the endpoint used to link the two. `LinkStripeCustomer` called the
legacy `PUT /v1/customers/{id}/billing-config/{type}`, whose request has no
delivery-method field. Metronome resolves the Stripe credential through the
delivery method, so the resulting record has no path to a token, and the error
names the token because that is where the lookup ends.

The record is also invisible to the current API. The legacy read returned a
configuration for every customer while `getCustomerBillingProviderConfigurations`
returned an empty list for all of them. Nothing in the environment reported a
problem: `listConfiguredBillingProviders` returns the Stripe connection whether or
not any customer can use it.

## Design

`LinkStripeCustomer` now performs the two writes Metronome requires, both
idempotent so a repeat card add is a no-op:

1. **Customer configuration.** `setCustomerBillingProviderConfigurations` with the
   environment's Stripe `delivery_method_id`, read at call time from
   `listConfiguredBillingProviders`. Exactly one Stripe delivery method must
   exist. Zero or several is an error rather than a guess, because with
   multi-entity billing the choice decides which Stripe account is billed.
2. **Contract routing.** A contract provisioned from a package carries an empty
   `billing_provider_configuration_schedule` and keeps its invoices inside
   Metronome. Each covering contract without one is edited to reference the
   configuration, effective `START_OF_CURRENT_PERIOD` so the open invoice routes
   too rather than waiting for the next period.

### Integration failures are now visible

Metronome emits `invoice.billing_provider_error` and `integration.issue` for
delivery and credential failures. Both need no configuration beyond a webhook
destination, and neither was handled, so a two-day delivery outage produced no
log line. They are not billing signals: account status is unaffected, and routing
them through the signal map would move an account's state on a delivery problem.
They are logged at error level with the provider's own message, ahead of the
billing-store guard so the alarm survives on a backend with no billing status.
`MetronomeWebhookArgs` gains a `Detail` field for that message, outside the
`river:"unique"` set so redelivery still dedupes on the event ID.

## Migration

No action for users. Accounts linked before this change have a configuration the
current API cannot see, and their invoices will not deliver. Re-run the link for
each by calling `setCustomerBillingProviderConfigurations` and attaching the
result to the covering contract, or have the account remove and re-add its card.
