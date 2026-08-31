# Dismiss traces from the review queue

## Summary

The review queue's only exit was **Add to dataset**, so an unwanted trace kept
reappearing on every load. This adds dismissal, with undo.

## Design

```http
POST   /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
DELETE /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
```

A dismissal is one row in `eval_dataset_dismissed_traces`, checked alongside dataset
membership wherever the queue or `POST /dataset/evaluations` filters candidates.
Membership and dismissal are mutually exclusive, enforced in SQL: admission clears
the dismissal in the same transaction that writes the item.

Both mutations are idempotent (200); dismissing a dataset item returns 409. In the
queue, **Remove** sits beside **Add to dataset** and reuses the same undo toast.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. Adds
`eval_dataset_dismissed_traces`. Additive, no backfill.
