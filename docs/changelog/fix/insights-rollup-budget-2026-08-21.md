# Insights roll-up finishes its backfill

## Summary

A cold account never completed its roll-up. The job planned the full 90-day
backfill, got about 28 days in, hit River's one-minute default job timeout, and
retried from the beginning. It repeated that until the retry budget ran out,
having stored nothing.

Two things combined to cause it. The job's deadline was never sized for its
work: rolling one day costs a few upstream round trips, so 90 days is minutes
long, and nothing had set a timeout. And the watermark advanced only after every
planned day committed, so a run that expired partway discarded the days it had
already paid for.

The failure was also invisible. The write that records why a run failed reused
the job context, which is the context whose deadline had just expired, so it
failed too. `consecutive_errors` stayed at zero and `last_error` stayed empty
for exactly the accounts that were stuck.

## Design

**The deadline matches the work.** The per-account worker declares a 15-minute
timeout. A roll-up is a batch job with no reader waiting on it, so the budget
can be generous. This also makes the existing per-day fetch cap meaningful: a
60-second bound on one day's upstream queries does nothing while the whole job
gets 60 seconds, and bounds a wedged day once the job gets minutes.

**Progress is durable per day.** The store gains `RecordProgress`, which moves
the watermark and leaves the error columns alone. The worker calls it after each
day's facts commit, so a run that dies keeps what it finished and the next
attempt starts where it stopped.

`RecordProgress` is separate from `Advance` rather than a flag on it, because
they mean different things. `Advance` also clears `last_error` and resets
`consecutive_errors`, and partial progress has not earned that reset. Folding
them together would pin the error counter at zero for an account that fails on
the same day every run, which is the case the counter exists to surface.

The watermark stays monotonic. `RecordProgress` upserts through `GREATEST`, so a
reconcile walking the window from below cannot rewind the stored value.

**Failures are recorded off the job deadline.** The state write after a failure
runs on a context detached from the job's, with its own short timeout. The most
common failure is the deadline itself, so the bookkeeping write cannot share it.

**Retries are bounded.** The per-account job caps at five attempts instead of
River's default 25. A roll-up that has not succeeded in five tries is waiting on
a fix, and the daily tick re-enqueues it anyway.

## Migration

None. The fact table and the state table are unchanged, and every write is
already a full replace, so stalled accounts converge on their next scheduled
tick without operator action.
