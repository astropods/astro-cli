# Stop auto-syncing eval datasets

## Summary

The v1 eval-dataset sync copied every production trace into a Langfuse dataset with the agent's own output as the expected answer. That produced noisy, drifting datasets with no quality signal. This branch stops creating more v1 data and prepares the dataset row for the judged dataset flow described in `docs/01-spec/eval-dataset-v2-spec.md`.

## Design

This branch removes the v1 sync path:

- The 24-hour dataset sync scheduler and both dataset sync workers are removed.
- `POST /api/v1/deployments/:id/dataset/sync`, `Queue.InsertDatasetSyncJob`, and the Eval tab **Sync** button are removed.
- Sync bookkeeping reads are removed: `last_trace_at`, `last_sync_attempted_at`, and `last_synced_at` are no longer selected, written, returned by `GET /dataset`, or shown in the client.

Deploy-time dataset provisioning stays in place, but new rows now point at `eval-{deployment_id}` instead of `dep-{deployment_id}`. Existing `dep-*` rows are lazily healed on the next deploy: the server creates the empty `eval-*` Langfuse dataset, repoints `eval_datasets.langfuse_dataset_name`, and resets `item_count` to `0`. Heal failures are swallowed so deploys never fail because of dataset state; the next deploy retries.

Dataset downloads still read from the dataset name stored in `eval_datasets`. The generated zip and JSONL filenames now use that same name, so healed and newly-created datasets download as `eval-*` files.

## Migration

None for users in this branch. The unused `last_trace_at`, `last_sync_attempted_at`, and `last_synced_at` columns can be dropped from `eval_datasets` next.
