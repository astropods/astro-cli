# Dataset Sync Reliability

## Summary

Dataset sync could fall out of step with Langfuse when a sync partially wrote dataset items but failed before Astro updated its local checkpoint and summary. A stale checkpoint then caused later manual or scheduled syncs to replay the same traces, which could create duplicate Langfuse dataset items and leave the Astro dataset endpoint reporting stale counts.

## Design

The sync is now idempotent at the Langfuse dataset-item layer. Astro supplies a deterministic dataset item `id` derived from the Langfuse dataset name and source trace ID. Langfuse upserts by that ID, so rerunning the same trace updates the existing dataset item instead of creating another row. This keeps dedupe inside Langfuse and avoids an Astro-owned dedupe table or a pre-read of all dataset items.

Writes remain best effort. Root-observation lookup and dataset-item writes log failures where they occur, then the worker continues with the next trace. Transient write failures are left for a later sync to retry, while permanent validation failures are treated as processed so one invalid trace cannot block the dataset cursor. A retried sync can safely repair missed items because the deterministic ID is stable.

The worker now skips traces that are not useful as dataset items before doing additional Langfuse work; today that means null-input traces. It also emits structured start, page-failure, and finish logs tagged with deployment, job, attempt, trace counts, upsert counts, item count, and the final checkpoint timestamp.

Once the local dataset row has been resolved, the worker attempts finalization even if a later trace-page read fails. Finalization refreshes and persists the Langfuse item count after any dataset-item write attempt or when Astro's local summary looks missing, writes the latest processed checkpoint timestamp when one is available, records the attempt timestamp, and only advances the successful sync timestamp when the job completes without error. This narrows the window where Langfuse has accepted writes but Astro still reports that no sync occurred.

The dataset row now separates sync attempts from successful syncs. A new `last_sync_attempted_at` column records that the worker reached finalization for the dataset, including partial runs that wrote or counted Langfuse data before returning an error. The existing `last_synced_at` column is preserved as the last fully successful sync time. This avoids overloading one timestamp to mean both "we tried and reconciled summary state" and "the job completed successfully."

The dataset scheduler continues to run on a 24-hour cadence. The per-deployment sync job keeps its uniqueness guard, so overlapping manual or scheduled syncs do not create duplicate active sync jobs.

## Migration

No user action is required. Atlas adds `last_sync_attempted_at` as nullable, so existing dataset rows continue to read normally and populate the field on their next sync attempt. Existing duplicate Langfuse dataset items created before deterministic IDs are not automatically removed; polluted datasets should be rebuilt or replaced if downloads need to exclude old duplicate rows.
