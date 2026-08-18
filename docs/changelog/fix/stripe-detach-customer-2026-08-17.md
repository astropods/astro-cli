# Map a detached card back to the account that held it

## Summary

A removed card is never recorded, so the account keeps the exemption a card
grants it.

`payment_method.detached` fires after Stripe has unlinked the method, so
`customer` on the event object is null. The handler read only that field and
enqueued an empty customer id. The worker then asked Stripe for that customer's
cards and got a 400 every time:

```
stripe list payment methods: parameter_invalid_empty
"You passed an empty string for 'customer'."
```

The answer never changes, so the job retried its full 25 attempts on an
`attempt^4` backoff spanning weeks.

Nothing user-facing broke while every removal came through the app, because the
card handlers write `has_payment_method` inline and a replacement is settled by
the `attached` event that accompanies it. The gap is a detach that happens
elsewhere: a card deleted from the Stripe dashboard, or a method Stripe prunes.
Nothing records it, `has_payment_method` stays true, and `credits_exhausted`
stops gating an account that has no way to pay.

## Design

**The previous customer is the only id the event carries.** Stripe puts the
cleared value in `previous_attributes`, so the handler falls back to it when the
object's `customer` is empty. The object still wins when both are present: an
`attached` event carries the current customer, and preferring the previous one
would attribute a replacement to whatever the field held before.

**An unresolvable card event is acked, not retried.** A card signal with no
customer names no account, and asking Stripe about an empty customer fails
identically on every attempt. The worker logs it and returns, because retrying
for weeks resolves nothing and buries real failures in the same queue.

The event is still enqueued when neither field holds an id. Refusing at the edge
would make Stripe redeliver an event nothing can ever resolve, and the record of
its arrival is worth keeping.

## Migration

None. Jobs already stuck in `retryable` for this reason stay until they exhaust
their attempts or are cancelled; the sweep does not need them.
