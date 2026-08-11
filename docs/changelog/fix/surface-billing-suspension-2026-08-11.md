# Say when an agent was stopped by billing

## Summary

A billing-suspended agent looked like it was starting up. The tiles read
"Starting", the status toggle read a green "Active" the owner could flip, the
chat composer offered "Start the agent to chat", and nothing anywhere said
billing had stopped it. The account banner said "Your agents are stopped" while
the agent page said the opposite.

The root cause was on the server. `GET /deployments/:id/status` has a switch
over the authoritative DB statuses, and `suspended` was not in it, so a
suspended deployment fell through to the branch that trusts DB-active and
answered `value: "active"`, `reason: "ready"`, `details: "Deployment is
active"`. Every client that branches on that response was told the agent was
healthy; the pod tiles then saw zero containers and applied their default,
which is "starting".

## Design

**Suspended is a status the coarse endpoint reports.** It resolves to
`inactive` with a new stable reason code, `suspended`, alongside the existing
`paused`. `inactive` rather than `error` because the deployment is deliberately
off, and a reason code rather than a new value because that is how the endpoint
already separates "why" from "what" for client branching.

**The client reads the reason code, not the record status.** The record carries
the loose string from `dbStatusToUIStatus`, which reports every status that is
not running, paused, or undeploying as `"error"`, so it cannot distinguish a
billing stop from a failed deploy.

**Suspended is a distinct client state, not another kind of paused.** It gets
its own `PodStatus` and outranks `paused` in `resolvePodStatus`, because both
mean zero replicas but only one of them the owner can undo. The toggle reads
"Suspended" and is disabled: a wakeup would be re-suspended on the next
recompute. The chat composer gets a matching state, since its stopped copy
tells the owner to start the agent, which is the one thing the account cannot
do until billing is resolved.

**One copy table for the gating reasons.** Only `credits_exhausted` implies no
card on file. Telling the other reasons (`payment_failed`, `balance_alert`,
`uncollectible`) to add a payment method is wrong, because the account already
has one. `lib/billing-copy.ts` maps reason to copy for both the account banner
and the per-agent tooltips, so the two can never give conflicting instructions
about the same suspension.

**The server is the authority on why.** `StatusUpdate` gains `EventDetails`,
written to `deployment_events.details` only. That is deliberately not
`ErrorDetails`, which also lands in `deployments.error_details` and would
misfile a billing stop as an error. The suspend worker resolves the account's
gating reason once per job, not once per deployment, and records it as a code:

```json
{"source": "billing", "reason": "credits_exhausted"}
```

An unreadable reason records `unknown` rather than defaulting to a specific
one, so the client falls back to copy that holds whatever the reason turns out
to be. Resume writes the matching event. Both rows previously had a null
message, so a billing stop was indistinguishable from any other transition on
the deployment timeline.

## Migration

None. `EventDetails` falls back to `ErrorDetails` when unset, so every existing
caller behaves exactly as before. Clients that predate the `suspended` reason
code ignore it and fall back to their generic inactive handling.
