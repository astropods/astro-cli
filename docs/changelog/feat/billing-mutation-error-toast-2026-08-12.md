# Say why a refused agent toggle failed

Stacked on the branch ahead of it in the server-owned-gating series.

## Summary

The agent status toggle called `mutate` with no `onError`. A refused wakeup
cleared nothing and said nothing: the switch settled back and the owner got no
explanation. Now that the operate routes gate on billing, that silence is the
first thing a gated account meets.

## Design

Both mutations share one `onError` that clears the pending intent and raises a
toast through the app's sonner wrapper.

The message is the server's. `getApiErrorMessage` prefers `details`, and a
billing 402 puts the actionable sentence there. The client composes nothing, so
the toast cannot contradict the 402 it came from.

Nothing here branches on billing. An earlier draft did two extra things and
neither survived contact with the code. It rebuilt the sentence from the gating
`reason`, which only repeated what `details` already said. And it redirected a
billing 402 to billing settings, which `BillingStatusBanner` already offers: the
banner sits in `Layout` above every routed page, so a gated account is looking at
that call to action while it clicks the toggle.

Without a redirect there is nothing to discriminate, so no change to the shared
`ApiRequestError` is needed and no unrelated quota page is touched.

## Scope

The deploy path already surfaced this. `useDeployForm` renders `apiErr.details`
for any failure, so a gated deploy has always shown the server's sentence. The
toggle was the gap.

The switch is disabled once a deployment itself reads billing-suspended, so the
reachable refusal is a stopped deployment whose account was gated afterwards.

## Migration

None.
