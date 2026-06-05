## Summary

Adds an Eval tab to the agent detail UI, backed by a new server-side dataset management system. Each deployed agent gets a Langfuse evaluation dataset provisioned immediately when the deployment goes active, then kept current via a nightly River background job. Users can view dataset metadata, trigger a manual sync, and download the full dataset as a JSONL archive.

## Design

**Data model** — A `deployment_datasets` table tracks one dataset per deployment: Langfuse dataset name, item count, last synced trace timestamp (`last_trace_at`), and last completed sync timestamp (`last_synced_at`).

**Eager provisioning** — `DeployWorker` fires a `provisionDataset` goroutine after a deployment goes active. It fetches Langfuse credentials, calls `CreateDataset`, and inserts the DB row — so the Eval tab is populated immediately without waiting for the nightly sync. Both `DeployWorker` and `DatasetSyncWorker` share an `ensureDataset` helper that handles create-if-not-exists idempotently; the sync worker's call acts as a fallback for deployments predating this feature.

**Sync pipeline** — Two River jobs handle ongoing sync:
- `dataset.sync_scheduler` runs nightly and fans out one `dataset.sync` job per active deployment.
- `dataset.sync` fetches traces from Langfuse tagged with the deployment ID, resolves the root observation ID for each trace, and upserts items into the Langfuse dataset. `MarkSynced` runs at the end of every job to record `last_synced_at`. Job uniqueness excludes terminal states so re-runs are always possible after a job completes.

**API** — Three endpoints under `/api/v1/deployments/:id/dataset`:
- `GET /dataset` — returns dataset summary (name, item count, last_trace_at, last_synced_at) from the local DB; 404 if not yet provisioned.
- `POST /dataset/sync` — enqueues an immediate sync job; returns 202; no-op if already queued.
- `GET /dataset/download` — streams a zip archive containing a JSONL file with all items fetched live from Langfuse.

**UI** — A new **Eval** tab (FlaskConical icon) in the agent detail bar shows dataset name, item count, and last synced time. Header buttons appear once data is present: **Sync** (queues a manual sync, shows a 1 s "Scheduled" confirmation) and **Download** (triggers the zip download). The tab is gated behind an `evals` experiment toggled in Settings → Experiments.

## Migration

No action required. The `deployment_datasets` table is created automatically via Atlas on server startup. Datasets are provisioned the next time a deployment is applied; existing active deployments will be backfilled on the next nightly sync run.
