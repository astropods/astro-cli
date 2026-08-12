# Hold the billing signals to a declared specification

## Summary

`ApplySignal` decides, for every provider webhook, which collection flags get
written. It was 28.6% covered and every flag writer but one was at 0%. A signal
wired to the wrong clear still recomputes to a plausible status, so the existing
tests, which assert the recomputed status, could not tell a correct mapping from
a wrong one.

Two latch bugs shipped from that gap: credit exhaustion and the spend alert
could each be raised by a webhook but not lowered by one.

## Design

**A declared spec, held to the SQL.** `signalWrites` states what each signal may
write, in order. The table is asserted against `sqlmock` statement by statement,
so it cannot describe a system we do not run, and an extra or reordered write
fails. Exhaustiveness comes from `billing.AllSignals`: a new signal with no spec
entry fails rather than reaching `ApplySignal`'s default at runtime.

**Pair by cause, not by column.** The obvious invariant, "every flag that is set
is cleared by something", is too weak to catch the bug it looks like it catches.
`SignalVoided` has always cleared `alert_active`, so a spend alert with no
resolved event of its own would have passed. The spec instead names each
raising signal's `inverse` and asserts that signal lowers the same flag.
Removing `SignalAlertResolved` reproduces the original failure:

```
alert raises alert_active with no inverse: only an operator could un-gate the account
```

**Reachability is the other half.** `credits_exhausted` did have a clearing
signal; nothing but our own provisioning job emitted it. That check needs the
provider event maps, which live in `riverqueue`, so it is asserted there: every
gate-clearing signal must be produced by some real event. Deleting the resolved
credit case reproduces that failure too.

Coverage of the package moves 35.9% → 60.7%, and `ApplySignal` 28.6% → 85.7%,
but the point is the two invariants, not the number.

## Migration

None. Tests and one exported `AllSignals` slice.
