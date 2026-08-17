# Charge what an account owes when it fixes its card

## Summary

An account suspended for a failed payment stayed suspended after the owner
added a working card.

Suspension for non-payment clears on one thing: `SignalRecovery`, which arrives
with Stripe's `invoice.paid`. Saving a card sends `SignalCardAdded`, which
records that a card exists and nothing else. The dunning timestamp survives, so
`computeStatus` still returns suspended.

Nothing then asked Stripe to charge the new card. Stripe retries a failed
invoice on its own smart-retry schedule, which can be days out or already
exhausted. Until it did, the 402 kept telling the owner to update their card,
they had, and their agents stayed at zero replicas.

The credits-exhausted path never had this problem. It gates on
`creditsExhausted && !hasPaymentMethod`, so the card alone lifts it inside the
request.

## Design

**Saving a card enqueues a charge for what is already owed.** `applyCardSignal`
adds a `billing.collect` job when the recomputed status is anything but active.
Active means no collection flag is raised and there is nothing to chase, which
is where the credits-exhausted account lands.

The job charges the customer's open invoices through a new provider capability:

```go
type invoiceCollector interface {
	CollectOpenInvoices(ctx context.Context, customerID string) (int, error)
}
```

It is an interface assertion rather than a method on `payment.Provider`,
following `stripeLinker` and `cardReader`. Only Stripe implements it, and the
seam stays as small as the thing it describes.

**Only `open` invoices are eligible.** A draft is not finalized, and paid, void,
and uncollectible are settled. Stripe refuses a pay call on all of them, so the
list query does the filtering rather than the loop. An uncollectible write-off
is therefore untouched by a card save, which is correct: it clears on a void.

**A decline is an outcome, not a fault.** A failed charge is counted and
skipped. Stripe emits `invoice.payment_failed` for it, and the existing webhook
path records the state. A failure to reach Stripe at all is different, and it
returns so River retries.

**The job writes no status.** Collection succeeds by producing `invoice.paid`,
which the webhook worker already turns into `SignalRecovery` and a resume. A
payment that lands outside our window reaches the same code, so dunning clears
in one place rather than two.

**Off the request path.** Charging a card is seconds of provider latency, and
3DS can make it longer. The card save answers as soon as the card is vaulted.

Collection is unique by args for a minute, so a double-submit from the card form
cannot charge one invoice twice. It is not gated by `BILLING_GATE_ENFORCE`, for
the reason resume is not: the invoices are real debt whether or not enforcement
is on, and the provider would charge them on its own schedule anyway.

## Migration

None. The job registers with the existing billing queue and needs no
configuration.
