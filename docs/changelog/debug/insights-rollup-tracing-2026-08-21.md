# Trace what an insights roll-up spends

## Summary

A roll-up that runs out of time reports only the query that happened to be in
flight when the deadline passed. That names a symptom, not a cause: the same
line appears whether the upstream is slow, the plan is too large, or the job
budget is too small. Nothing recorded how many days a run planned, how long
each took, or how much of the deadline was left.

## Design

The worker logs its plan before it starts: the watermark it read, the day count
and range, the attempt number, and the budget it has. A cold account planning
90 days against a one-minute budget is then visible in one line, before any
work happens.

Each day then reports its own duration alongside the elapsed total and the
remaining budget. A roll-up spends one deadline across every day it planned, so
the remainder is the number that explains a timeout; a single day's duration
never does. The existing failure warning carries the same fields, so the line
that reports the timeout also reports how far the run got.

Below that, each upstream query reports its grain, row count, and latency. The
grain label separates the two `traces` queries from the `observations` one,
which is what tells a slow upstream apart from too many queries.

Everything is `Debug`, so production keeps its current output at `info`.

The messages in this path had drifted into three shapes: `Insights rollup:`,
`Insights rollup skipped:`, and a bare `Insights rollup completed`. They now
follow the convention the `river:` logs already use, a lowercase component
prefix and an all-lowercase phrase, with detail in structured fields.

## Migration

None. Set `LOG_LEVEL=debug` to see the new lines.
