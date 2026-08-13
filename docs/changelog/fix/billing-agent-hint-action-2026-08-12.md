# Key the billing-stopped tooltip on the server's action

Stacked on the branch that adds `action` to `BillingStatusResponse`.

## Summary

`billingStoppedHint` was the last place the client decided what resolves a
billing gate. It switched on `reason` and told a spend-limited account
"Stopped by billing. The account hit its spend threshold", which names the
problem and no fix. The server returns `contact_support` for that reason,
because only an operator can raise a spend limit.

The tooltip on a stopped agent therefore left the owner with nothing to do,
while the 402 for the same account told them to contact support.

## Design

The hint is now a map keyed on the server's `action`, matching how the banner's
button already works. `AgentStatusToggle` passes `billing?.action`.

The wording stays in the client. The server's `details` is account-scoped
("This account reached its spend limit"), and this tooltip sits on one agent
("to start it again"), so the sentence differs on purpose. What the client no
longer owns is the instruction inside it.

An unrecognised action falls back to the `view_billing` line, which holds for
whatever the action turns out to be.

## Migration

None.
