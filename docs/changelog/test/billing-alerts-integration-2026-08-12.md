# Integration coverage for the billing alert machine

## Summary

Every existing test of the gating machine uses sqlmock, which asserts the SQL we
send rather than what a database does with it. These six run the same paths
against a real Postgres.

## Design

The distinction matters in three specific places, and the cases are chosen for
them rather than for coverage.

**The latch columns share one row.** Each signal writes one flag through an
upsert. A signal that touches the wrong column, or an `ON CONFLICT` that clobbers
a sibling, passes a string assertion and loses a suspension. Credit exhaustion
with and without a card exercises two flags on one row.

**computeStatus ranks five reasons.** Only a round trip proves the rank the code
intends is the rank a stored row produces. The rank test latches three reasons at
once, asserts the write-off wins, then voids it and asserts the status drops to
the next reason still latched rather than to active.

**The dunning clock is a timestamp, not a string.** sqlmock can assert a
`COALESCE` appears in the statement. It cannot prove a redelivery keeps the first
delivery's timestamp, which is the difference between a fixed deadline and one
that walks forward on every provider retry.

`ApplySignal` stamps `dunning_since` with the time it is handed and evaluates the
grace window against that same instant, so the first delivery is always within
grace however far back it is dated. That makes the redelivery the assertion: it
is stamped now, `COALESCE` keeps the original, and the window it measures is
eight days wide, so the account suspends. A clock that restarted would measure
from now, span nothing, and leave the account past_due and still running unpaid.

The last case pins that a payment clears dunning and nothing else, so a
spend-limited account stays stopped. Period spend is unchanged by a payment, and
resuming there would be a wrong-direction un-gate.

## Running

`moon run astro-server:test-integration`, which brings up a migrated Postgres in
Docker. No Kubernetes needed.

Each case seeds its own account and drops it on cleanup, so the suite is
re-runnable and leaves no rows behind.
