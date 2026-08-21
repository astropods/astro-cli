# Insights roll-up finishes its backfill

## Summary

**The problem, plainly:** the Insights page never filled in for a new account.

The job that builds the page had to process 90 days of history. It asked
Langfuse for one day at a time, which meant about 360 separate network requests,
which took about three minutes. The job was only allowed to run for one minute.

So it got about 28 days in and ran out of time. And because it only saved its
place at the very end, those 28 days of finished work were not recorded. The
next attempt started at day 1 again, did the same 28 days, and ran out of time
in the same spot. Forever. It is a download with no resume that never finishes.

**The fix, plainly:** stop asking day by day. Langfuse can return a whole date
range broken down by day in a single request, so the job now asks once per
month-sized chunk instead of once per day. 360 requests become 12. Three minutes
becomes seconds, and the job finishes comfortably inside its time limit.

Two smaller changes make sure a slow run can never get stuck the same way again:
the job now saves its place as it goes, and it gets a time limit sized to its
actual work instead of the framework default.

## Design

**Fetch by range, write by day.** The producer's unit of *fetch* is now a window
of up to `MaxDaysPerWindow` (30) days; the unit of *storage* is still one day.
Langfuse's metrics API takes a `timeDimension` of `day`, so a range query
returns one row per day per group and the producer splits the response back
apart before writing. The read path already worked this way; the roll-up was the
only caller asking per day.

Two consequences worth knowing:

- The count and cost/tokens queries are joined back together by a group key, and
  that key now includes the day. Without it the same `(tags, userId)` on
  different days collides and every day's request count pairs with one arbitrary
  day's cost.
- `RollUpRange` takes an explicit list of days, not a pair of bounds. A range
  query only returns days that had activity, so writing just those would leave
  stale rows on a day whose spend dropped to zero. Every requested day gets its
  full replace either way.

**Progress is durable.** The store gains `RecordProgress`, which moves the
watermark and leaves the error columns alone. The worker calls it after each
window commits, so a run that dies keeps its finished windows.

It is separate from `Advance` rather than a flag on it because `Advance` also
clears `last_error` and resets `consecutive_errors`. Partial progress has not
earned that reset: folding them together would pin the error counter at zero for
an account that fails on the same day every run, which is the case the counter
exists to surface. The watermark stays monotonic either way, since
`RecordProgress` upserts through `GREATEST`.

**The deadline matches the work.** The worker declares a 15-minute timeout. A
roll-up is a batch job with no reader waiting on it, so the budget sits well
above the worst case rather than being tuned to it. This also makes the existing
per-window fetch cap meaningful: a 60-second bound does nothing while the whole
job gets 60 seconds.

**Failures get written down.** The state write after a failure runs on a context
detached from the job's, with its own short timeout. The most common failure is
the deadline itself, so the bookkeeping write cannot share it.

**Retries are bounded.** The per-account job caps at five attempts instead of
River's default 25. A roll-up that has not succeeded in five tries is waiting on
a fix, and the daily tick re-enqueues it anyway.

## Migration

None. Both tables are unchanged and every write is already a full replace, so
stalled accounts converge on their next scheduled tick without operator action.
