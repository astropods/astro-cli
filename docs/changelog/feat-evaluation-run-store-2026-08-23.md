# Evaluation run store

## Summary

The evaluator worker owned the whole lifecycle of an evaluation run: it created the run when
it picked up a job, then wrote results against it. Nothing else could see a run before the
worker started, so a requested trace had no state until a worker was free. This change moves
run creation ahead of the worker and adds the queries an API needs to read runs back. The
endpoints that call those queries arrive separately, so this change adds no routes.

## Design

A run is now recorded by whoever requests it, and the worker claims a run that already
exists. `CreateQueuedRuns` writes one queued run per trace in a single statement, and
`FailQueuedRuns` closes them out when the request cannot reach the queue. `StartRun`
replaces the worker's previous create-or-adopt call: it moves a queued run to in-progress
and returns nothing when no active run is there to claim. A worker that finds no run treats
the job as stale rather than inventing state, which keeps the run table a record of what was
actually requested.

The read side adds four queries that nothing in this change calls. They land here so the
endpoint change that follows holds only HTTP concerns, and each one has a single consumer
there:

| Query | Consumer |
|---|---|
| `StatusCounts` | `GET /dataset/evaluations/status`, deployment-wide totals |
| `LatestRuns` | the review queue listing, one run per row |
| `TracesWithCompletedRuns` | the review queue's `evaluated` filter |
| `EvaluatorResults` | `GET /dataset/review-queue/:trace_id/evaluation`, every result for one run |

A retry inserts a new run rather than replacing the old one, so a trace accumulates rows.
Every read reports the latest run per trace, matching what the queue listing already shows,
and one index serves all three:

```sql
CREATE INDEX eval_dataset_evaluation_runs_latest_idx
    ON public.eval_dataset_evaluation_runs
    (eval_dataset_id, trace_id, created_at DESC);
```

Preset evaluators also carry a `description` explaining what each one checks. The per-trace
breakdown returns it alongside each result, so the client labels an evaluator without
hardcoding copy per key. Nothing reads it yet either.

## Migration

None. No route changes, and no code path enqueues evaluation jobs yet. The schema change is
one added index.

A job inserted by hand for a trace with no recorded run now finishes without doing anything,
where before the worker created the run. Call `CreateQueuedRuns` first to record the run.
