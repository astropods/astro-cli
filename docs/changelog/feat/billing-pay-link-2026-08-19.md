# Let a customer finish a payment their bank stopped

## Summary

A charge that needs bank authentication could not be completed from the product.
Stripe sends no email for a `charge_automatically` invoice, so the hosted page is
the only route the customer has, and the app never showed it. The banner told
them to update a payment method that was working, and doing so changed nothing.

## Design

**The link lives exactly as long as dunning does.** `payment_action_required`
stores Stripe's hosted page on `account_billing_status.pay_link`, and
`ClearDunning` nulls it in the same statement that clears the marker. Tying the
two together makes the invariant structural rather than remembered: an account
that owes nothing cannot be offered a payment page, which would be a dead link to
a settled invoice.

**The link belongs to one invoice, and dunning does not.** The marker covers
every open invoice at once, so a link stored for the invoice that wanted
authentication would still be offered when a later, unrelated invoice declines.
Paying it would settle a real debt and leave the account stopped. Both Stripe
events carry the invoice's hosted URL, so `payment_failed` clears the link when
that URL differs from the stored one. The condition matters: a blanket clear
would also drop the link on a retry of the same invoice, which is the one case
the link exists for.

The comparison is against the empty string too, so an event that cannot name its
invoice clears the link rather than leaving it unvouched for. That is the safe
direction. Keeping a link risks charging for an invoice that is not the one
holding the account, while dropping one falls back to replacing the card, which
still resolves a decline.

**A write-off is not a collection reason for this purpose.** Marking an invoice
uncollectible sets `force_suspended` and clears neither the dunning marker nor
the link, so a written-off account can still hold one. Only a void or an operator
lifts that flag, and payment recovery deliberately does not, so offering the link
there would take the customer's money and leave the account stopped. Uncollectible
keeps `update_card`.

**A pay link outranks `update_card` on the remaining collection reasons.** When the bank
asked for authentication the card is fine. `BillingAction` gains a fourth answer,
`complete_payment`, chosen only when the account is in that state and holds a
link. Without one the same reason still means the card needs replacing, so a
build that has no link never offers a button that goes nowhere.

**The gate reads the record, not the status.** `Entitlements.Check` used a
narrower query that returned status and reason. A refusal has to name its fix,
and for this one the fix is a URL on the row, so it now reads the whole record
and carries the link on the `Decision`. The 402 body and the banner therefore
agree, which is the property the action field exists to guarantee.

**One action leaves the app.** Every other fix is on our billing page. This one
goes to Stripe's hosted page in a new tab, as a link rather than a `window.open`:
a blocked popup would leave the one control that unblocks the account doing
nothing visible. `ActionPanel` gains `primaryHref` for it. The banner branches on
the presence of the link rather than on the action string, so a build that
predates the action still behaves.

**The link is checked as a URL before it is stored.** It becomes an `href`, where
a `javascript:` scheme executes on click, so the writer accepts only an absolute
https page and the banner renders only one. Stripe signs the webhook that carries
it, which makes this the second lock rather than the first.

## Migration

`account_billing_status` gains a nullable `pay_link`. No backfill: the column
fills the next time a charge needs authentication.
