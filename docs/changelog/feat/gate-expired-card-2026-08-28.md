# Gate accounts whose card has expired

## Summary

A card expiring produced no reaction anywhere in the billing pipeline. Nothing read the expiry date: the server stored the month and year for display and never compared them to today, so `has_payment_method` stayed true and the account kept the pay-as-you-go floor that having a card buys it. The first sign of trouble was the declined charge after the period closed, which starts the seven-day dunning grace. An account could therefore spend a full period plus a week on a card that could never be charged.

Expiry is the only card change no provider announces. Attach, detach and a network reissue all arrive as Stripe events; the month simply passing does not. So the fact goes stale silently, and the only way to find it is to re-read the vault.

## Design

**An expired card counts as no card.** `payment.Card.Expired` puts the boundary at the first instant of the month after the expiry month, in UTC, because issuers accept a card through the last day of that month. A payment method carrying no expiry at all is never expired, so a non-card method is not dropped by a zero value.

`resolveCardSignal`, the one function that decides whether a customer has something chargeable, now applies that rule. Every existing caller inherits it, including the card fact refreshed when Metronome reports the signup credit spent.

**A daily sweep supplies the missing event.** `billing.card_expiry_sweep` walks the accounts recorded as carded, re-reads each default card, and applies `SignalCardRemoved` to the ones that have expired. Daily is enough: a card expires on a month boundary, and each tick costs one provider read per carded account. An account leaves the work set as soon as its fact is cleared, so the sweep only ever re-reads cards that still look valid, and it returns when a fresh card is saved.

The walk is keyset-paginated on `account_id`, and every page is read on every tick. Ordering on `updated_at` and taking one page would have been the natural shape, and it starves: a valid card writes nothing, so a healthy account's `updated_at` is frozen at card-add time, the same oldest page comes back every day, and every account behind it is never checked. Keying on the id makes coverage independent of what the rows have written lately.

**The sweep decides nothing about suspension.** It applies a signal and lets the state machine rule, which is what keeps the outcome consistent with a card that was never added at all:

- Inside the signup credit, the account owes nothing and stays active. Its gateway ceiling drops to the card-less default, which is the only thing that changes.
- Past the credit, `credits_exhausted` is already latched and the account suspends on it immediately, with the same reason and the same suspension notice a card-less account gets.

No new status, flag, reason or signal was added. The gateway ceiling is re-derived in the same pass rather than waiting for its own sweep, because the ceiling is enforced in real time and it is the control that actually stops the spend.

The suspend is enqueued before the ceiling, and neither enqueue can be skipped by the other failing. Clearing the latch takes the account out of every work set: it leaves the carded list, and no sweep scans a suspended row. A suspend dropped there is dropped permanently, while the ceiling re-derives on its own fifteen-minute sweep.

Recovery is unchanged: saving a card re-attaches, and Stripe's `payment_method.automatically_updated` covers the network reissuing an expiring card.

Public documentation for the behaviour lives on the Usage limits page: an expired card counts as no card, an account inside its signup credit keeps running, and one past it stops until a new card is saved.

## Migration

None. The sweep registers only where a payment provider is configured, and the first tick after deploy clears the fact for any account already holding an expired card.
