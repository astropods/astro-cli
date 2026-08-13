# Cover the dunning sweep and the grace boundary

## Summary

The dunning sweep decides when a failed payment stops being tolerated and the
account loses service. It had no test file. `ListInDunning`, which defines the
sweep's entire work set, had no coverage either.

`computeStatus` was well covered for how the gating reasons rank, but not at the
grace boundary. Every existing case sat two days inside or one day outside a
seven-day window, so the comparison itself was never exercised.

## Design

**The boundary is where a day of service is decided.** `now.Sub(since) > grace`
is strictly greater, so an account exactly at its grace expiry is still
past_due. Three cases pin that: exactly at grace, one second inside, one second
past. An off-by-one here either stops a paying customer a day early or serves a
non-paying one a day free, and neither shows up in a ranking test.

**The work set is a filter with two failure directions.** Widening
`ListInDunning` past `past_due` re-evaluates every stopped account on every
tick; narrowing it leaves a failed payment running forever. The test asserts the
status and limit reach the query as arguments. A second test covers a row error
mid-iteration, which must surface rather than return a short slice with a nil
error, because a silently truncated work set looks exactly like a clean sweep.

**One bad account must not stop the rest.** The sweep processes the whole work
set in a single job, so a locked row on one account cannot be allowed to skip
the accounts behind it. The test drives three accounts with a failing read in
the middle and asserts the third is still recomputed. Replacing the `continue`
with a `return` turns it red.

**The enqueue is the part worth proving.** The recompute only records that an
account aged out; `InsertBillingSuspend` is what takes the service away. Testing
it meant narrowing the worker's `*Queue` field to a two-method `dunningQueue`
interface, since a concrete `*Queue` cannot be faked. Two tests follow: a
transition enqueues the suspend and notifies the owner, and an already-suspended
account does neither, because re-firing on every tick would re-suspend and
re-notify hourly for as long as the account stays unpaid.

## Migration

None. Tests, plus a narrowed field type on an unexported worker.
