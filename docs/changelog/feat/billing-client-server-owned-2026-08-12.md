# Let the server decide whether an account is gated

## Summary

The web client recomputed the gating rule the server already applies. The banner
asked `enforced || workloads_suspended` and then checked the status itself, and
`billingBannerCopy` inferred credit exhaustion from raw flags when no reason
arrived. Two copies of one rule drift, and the drift shows up as a banner that
disagrees with the 402 the same account gets on a deploy.

The server has published `gated` and `action` since the gating-reason change, but
the client's `BillingStatusResponse` never gained the fields, so nothing consumed
them.

## Design

`BillingStatusResponse` now carries `gated` and `action`, and the banner renders
on `gated` alone. The server computes it as
`status != active && (enforced || workloads_suspended)`, which is exactly what
the banner used to assemble.

`billingBannerCopy` takes the reason, the action, and the suspended flag. The
inference it replaced is unreachable: `computeStatus` pairs every non-active
status with a reason, `Recompute` is the only writer of the pair, and `reason` is
NULL only alongside `active`. A gated account therefore always states a reason.

The reason picks the wording. The action picks the button, through one
`ACTION_LABEL` map keyed on the server's value.

### The button was contradicting the server

Deriving the button from the reason produced a live disagreement on
`balance_alert`. `middleware.BillingAction` returns `contact_support` for it,
because only an operator can raise a spend limit, and the 402 body says "Contact
support to raise it". The banner offered "View billing" and navigated to billing
settings, where nothing the user can do helps. The same account got two different
instructions depending on whether it hit the web app or the CLI.

The app has no support route, so `contact_support` keeps the billing destination
and the instruction moves into the body copy, which now matches the server's
`details`. That is recorded in the map rather than left implicit.

### Coverage moved rather than dropped

Two banner tests were the only coverage of the gating rule, one for observe mode
and one for a suspension that outlived enforcement being turned off. Deleting the
client-side rule would have deleted its tests with it, so the rule is now tested
where it lives, in `TestBillingStatus_GatedFollowsEnforcementAndSuspendedWorkloads`.
The banner tests keep the same two scenarios, restated as "follow the server's
verdict".

The test that asserted the inference now asserts the banner still speaks when a
gated status carries no reason. That shape does not occur today, and going silent
would hide a stopped account.

### Deploy skew

`deploy-prod.yml` selects services one at a time, so the client can ship ahead of
its server. Against a server that predates `gated`, the field is undefined, and
reading that as "not gated" would hide a real suspension. The banner falls back
to reconstructing the old rule for that case only. Delete the fallback once no
deployed server predates the field.

## Migration

None.
