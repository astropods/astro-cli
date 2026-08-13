# Drop the billing action that cannot happen

## Summary

`BillingAction` returned four values and one of them was unreachable. Five gating
reasons exist, `computeStatus` can return all five, and the switch covered all
five, so `view_billing` only fired for a reason the build had never heard of.

That made the vocabulary read as four peer outcomes when there are two the owner
can act on: no card on file, or a card that failed.

## Design

Three values remain. `add_card` and `update_card` are the self-serve pair, and
they stay separate because they differ in what a client renders: an empty card
form, or replacing a card already on file. Telling someone with a card to add one
is the confusion this mapping exists to prevent.

`contact_support` is the absence of a fix rather than a third one, and it now
absorbs the unknown-reason case. A build that cannot name the problem must not
send the owner to change a card that may be fine. The trade is support load on a
reason we should not be emitting, in exchange for never giving wrong advice.

The client's two maps follow, and their fallback moves from a `view_billing` key
to `contact_support` for the same reason.

## Why now

`BillingAction` takes a reason and nothing else. Customer-set spend limits break
that: `balance_alert` will mean two different things depending on whether the
customer or an operator set the threshold, and only one of those is self-serve.
Collapsing the taxonomy first keeps that change to one decision instead of two.

## Migration

None. `view_billing` was never emitted, so no client can have received it.
