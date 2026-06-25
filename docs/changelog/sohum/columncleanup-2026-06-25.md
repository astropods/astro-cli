## Summary

Removes the leftover eval dataset sync columns that predate the current human-judgment flow. The judged dataset path now treats Astro's local row as the place for dataset identity and scored-label counters only; trace content and item metadata remain in Langfuse.

## Design

The `eval_datasets` table drops the old sync-era storage fields: `item_count`, `last_trace_at`, `last_sync_attempted_at`, and `last_synced_at`. The timestamp fields were no longer selected, written, or returned. `item_count` had also become stale because the summary API already derives the public count from `good_count + bad_count`.

The server keeps the public `item_count` response field for client compatibility, but computes it from the local good/bad counters. Filtered dataset pagination now sizes its bounded Langfuse scan from those same counters instead of reading a stored item total.

`eval_dataset_judgments.created_at` stays in place. The judgment table is still the duplicate gate for all verdicts, including neutral/unknown, and `created_at` is the only local audit timestamp for those rows.

## Migration

No user action is required. Operators applying the schema diff can drop the unused `eval_datasets` columns without backfilling because the live summary is derived from `good_count` and `bad_count`.
