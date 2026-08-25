# Confirm a card removal, and say why it was refused

## Summary

Removing a payment method took one click and no confirmation. The card paying
for running agents detached immediately, and the account dropped to the free
tier with its workloads stopped.

The server now refuses a removal that leaves a balance uncollectable, and
answers 409 with the reason. The old handler discarded it:

```ts
onError: () => toast.error("Couldn't remove payment method"),
```

An account told to settle its balance saw a generic failure that reads like a
network blip, and no path to the fix.

## Design

Removal is a downgrade, not a way to change cards. Saving a new card already
detaches the old one in the same call, so the confirmation says so and links to
that flow instead of leaving someone stuck behind a refusal they cannot act on.

The dialog reuses `ConfirmationDialog`, which renders `error.message`. Since
`ApiRequestError` carries the server's `error` field, the 409 explains itself
without the component knowing anything about balances.

The mock backend had no billing routes at all, which is why the billing UI had
no browser coverage. It now serves the payment-method read and delete, with a
`/test/set-billing-owed` toggle that mirrors the server's 409, so the refusal
path is exercised end to end rather than assumed.

## Migration

None.
