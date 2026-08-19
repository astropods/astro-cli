# Bill each stretch of compute exactly once

## Summary

An account could be charged twice for the same minutes. The metering heartbeat
billed the span from a stored anchor to now, advanced the anchor only after a
successful ingest, and gave every event a fresh transaction ID. When the anchor
failed to advance after usage had already reached Metronome, the next tick
re-billed the same span, and nothing could tell the two apart.

Measured on the current code: 0.25 CU-hours charged for 0.1667 hours of
reservation, a 50% overcharge on the affected window.

## Design

Two things have to be true for a repeat to be harmless. Metronome must recognise
it, and the repeat must be identical.

**Usage is billed on a fixed window.** Time is divided into five-minute windows
aligned to the clock. A tick emits one event per window that has fully closed,
and the transaction ID is the row plus the window start. Metronome ignores a
transaction ID it has already accepted, for 34 days, so re-emitting a window is
free.

**Only closed windows are emitted.** A window still open has no final value: the
tick that saw it half elapsed and the tick that saw it whole would send different
amounts under one ID, and Metronome would keep the first. Waiting for the window
to close makes the repeat byte-identical. The cost is that usage is billed up to
one window later than before. Nothing is lost, because the anchor still moves
only over windows that were emitted.

**The anchor stops being load-bearing.** It was the only thing preventing a
double charge, and it was written in a separate statement that could fail on its
own. It now decides when work is repeated rather than whether it is correct.

**Events are stamped with the end of the span they cover** rather than the moment
they were sent. Metronome files usage into a billing period by event time, so a
catch-up emit would otherwise bill last night's usage to today.

**Catch-up is bounded** at 24 hours of windows per row per tick. The anchor
advances to the last window emitted, so the next tick continues where this one
stopped.

**Catch-up stops at the backdating limit.** Metronome rejects usage timestamped
more than 34 days ago, and the anchor advances past whatever is emitted, so a row
further behind than that would lose its oldest hours silently. Those windows are
skipped deliberately and logged at error level with the unbillable duration,
because revenue that cannot be recovered should not be a quiet log line.

**The legacy heartbeat path is deleted.** It billed a fixed interval under a
fresh UUID, which is the pattern this change removes, and it ran only when no
provider was configured, where its first ingest would have hit a nil interface.
Nothing in production reached it: the six tests that broke on its deletion tested
only itself.

The two closing paths, a deployment that went stale and one that recorded a stop
time, bill up to a fixed timestamp rather than the window grid. Their IDs come
from both ends of that span, which are equally deterministic.

Both architecture docs claimed the opposite of the truth: that the per-event UUID
made retries and backfill safe. They are corrected alongside the code, since
believing that claim is what makes the bug invisible on review.

## Migration

None. Existing anchors are read as they are, and the first tick after deploy
bills whole windows from wherever each one sits.

Two existing tests changed, because they asserted the old semantics rather than
the intent. One expected the anchor to advance to `now`; it now expects the last
closed boundary. The other proved that healing a missing row leaves an existing
anchor alone, using "close to now" as the proxy for "moved"; it now asserts the
anchor moved past the seeded value without passing now.
