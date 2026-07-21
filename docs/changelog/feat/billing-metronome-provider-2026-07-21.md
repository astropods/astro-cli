# Stripe card vault for payment methods

## Summary

Accounts can now save a real credit card. Previously the billing page showed a
demo-only payment-method form that stored nothing. This adds a Stripe-backed card
vault: the card is collected client-side with Stripe Elements, saved to a Stripe
customer, and linked to the account's Metronome customer so Metronome charges the
saved card when it finalizes invoices. astro-server never moves money — it only
collects and stores the card.

## Design

**Two providers, two concerns.** Metering (usage → invoices) stays on the
existing `billing.BillingProvider` seam. Card collection is a separate
`payment.Provider` seam (`internal/payment`) with a Stripe implementation. Both
are nil-guarded and default off; the payment provider is enabled only when
`STRIPE_SECRET_KEY` is set.

**Card-only, no webhook.** Because `confirmSetup` resolves synchronously for
cards, setup is confirmed on a normal request rather than a webhook:

1. `POST /billing/setup-intent` ensures a Stripe customer (persisted on
   `accounts.stripe_customer_id`) and returns a SetupIntent client secret +
   publishable key.
2. The client confirms the card with Stripe Elements (card data never touches
   our server; SCA runs up front via `usage=off_session`).
3. `POST /billing/payment-method` re-reads the SetupIntent from Stripe
   (authoritative — the client is not trusted), verifies it succeeded, makes the
   card the customer's sole default, and links the Stripe customer to Metronome.

There is no `STRIPE_WEBHOOK_SECRET` and no inbound Stripe endpoint.

**Metronome linkage is an optional capability.** The confirm handler links via
an interface assertion (`LinkStripeCustomer`, backed by Metronome's billing
config with `charge_automatically`) so the core `BillingProvider` interface stays
metering-only. A link failure is logged, not fatal — the card is already vaulted
and the link can be retried on the next save.

**Frontend.** The demo `PaymentMethod` form is replaced by a Stripe Elements
flow (`@stripe/react-stripe-js`), with `usePaymentMethod` / `useConfirmPaymentMethod`
/ `useDeletePaymentMethod` query hooks. The section hides itself when payments
aren't configured for the environment.

## Migration

No user action required. New env vars are optional:

| Variable | Purpose |
|---|---|
| `STRIPE_SECRET_KEY` | Enables the card vault (server-side Stripe key) |
| `STRIPE_PUBLISHABLE_KEY` | Surfaced to the client for Stripe.js/Elements |

A new nullable `accounts.stripe_customer_id` column is added (deployed via
Bytebase). When Stripe is unset, billing behaves exactly as before.
