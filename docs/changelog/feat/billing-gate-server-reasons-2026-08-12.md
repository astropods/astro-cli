# Say why a request was refused for billing

## Summary

The billing gate returned a 402 that named no reason:

```json
{"error":"Billing suspended","code":"BILLING_SUSPENDED",
 "details":"... Update your payment method to continue."}
```

The fixed `details` is wrong for the most common case. An account gated on
`credits_exhausted` has no card to update; it needs a first one. That is the
same copy bug the web banner already fixed by branching on the reason code, and
the gate had no way to.

`Entitlements.Blocked` discarded the reason it already read, so no caller could
do better. The CLI, which prints the response body verbatim, could only show the
wrong sentence.

Waking a suspended agent was refused for the wrong reason entirely. A
billing-stopped deployment holds `StatusSuspended`, so it failed the
`StatusStopped` check and returned `400 "deployment is not stopped"`: true,
useless, and it hid the only fact the caller could act on.

## Design

**The gate names the fix, the surface writes the words.** `Check` returns a
`Decision` carrying the reason, and `BillingAction` maps it to one of
`add_card`, `update_card`, `contact_support`, or `view_billing`. The 402 body
carries the reason code, the action, and a plain sentence.

Rendered copy is deliberately not in the response. A terminal and a banner
phrase things differently, and neither should own the decision about which fix
applies. The plain sentence exists only so a client that prints the body as it
stands still says something true, which is what the CLI does today.

An unrecognised reason maps to `view_billing` rather than guessing, so a client
older than a new gating reason degrades to something honest.

**Wakeup is gated after membership and before the status check.** The reason
names another tenant's billing state, so authorization has to be settled first;
otherwise any authenticated caller holding a deployment id learns whether that
account is suspended and why. The status check stays behind both, so a billing
stop reports as one instead of as "deployment is not stopped".

Moving the membership check ahead of the status check closes a narrower
pre-existing disclosure as well: a non-member could previously read a
deployment's status from the 400. `TestWakeUpDeployment_NotStopped` only passed
because of that ordering, and now sets up the membership query it always should
have.

**The status endpoint gained `gated` and `action`.** `gated` is
`status != active && (enforced || workloads_suspended)`, which the client was
computing itself; `action` matches the 402 body so a banner and a refused
request cannot disagree about the fix.

**`Wrap` logs when it finds no account in context.** It allows in that case,
which is right, but silently: the route reads as gated at the call site and
gates nobody.

## Migration

None. `details` is still present and still a string. Clients that ignore the new
fields behave as before. The gate remains inert while `BILLING_GATE_ENFORCE` is
false, which is the default.
