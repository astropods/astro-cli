# Consistent AI judge state

## Summary

Keeps the AI judge action and progress state accurate when the review queue is filtered by verdict.

## Design

Prediction lifecycle counts are exposed separately from the filtered review queue. Completed prediction output takes precedence over a stale request status, and traces with human judgments are excluded.

The client polls the shared status while work is active and refreshes the selected queue as predictions change. Other filtered queues are marked stale and reload only when selected. Active work always disables the judge action; the “nothing left to judge” state is derived from loaded items only in the unfiltered queue. The previous queue remains visible while a newly selected filter loads, avoiding an empty-state flash between cached views. The judging indicator uses a persistent reduced-motion-aware animation that remains active through filter renders.

## Migration

No migration is required.
