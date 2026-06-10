# Eval Infrastructure

## Phase 1: Deployment Datasets

### Summary

To evaluate an agent, you need a dataset of real inputs and expected outputs to test against. Without a dataset, there is nothing to run an evaluation on. Phase 1 solves this by automatically building a dataset for every deployment from its production traces — each trace becomes a dataset item with the real input and the agent's actual output as the expected baseline. This gives teams a ready-made regression dataset without any manual curation.

A dataset is provisioned immediately when a deployment goes active. A daily River scheduler job keeps each dataset current with new traces. Users can view dataset metadata, trigger a manual sync, and download the full dataset from an Eval tab in the agent detail UI.

---

### Data Model

One new table: **`deployment_datasets`** — one row per deployment, keyed by `deployment_id`.

| Column | Type | Notes |
|--------|------|-------|
| `deployment_id` | `varchar(11)` | PK, FK → deployments |
| `account_id` | `uuid` | FK → accounts |
| `langfuse_dataset_name` | `varchar` | `dep-{deployment_id}` |
| `item_count` | `int` | running total of synced items |
| `last_trace_at` | `timestamptz` | `createdAt` of newest trace synced; null = never synced |
| `last_sync_attempted_at` | `timestamptz` | timestamp of last finalized sync attempt; null = never attempted |
| `last_synced_at` | `timestamptz` | timestamp of last fully successful sync job; null = never synced |
| `created_at` | `timestamptz` | |
| `updated_at` | `timestamptz` | |

---

### Langfuse Dataset API

New methods on `*Client` using the same Basic auth as existing trace calls:

| Method | Endpoint |
|--------|----------|
| `CreateDataset(ctx, name, description string) error` | `POST /api/public/v2/datasets` |
| `UpsertDatasetItem(ctx, item DatasetItemInput) error` | `POST /api/public/dataset-items` |
| `GetDatasetItems(ctx, datasetName string, page, limit int) (*DatasetItemsResponse, error)` | `GET /api/public/dataset-items?datasetName={name}` |

`DatasetItemInput` fields written by sync: `ID`, `DatasetName`, `Input`, `ExpectedOutput`, `Metadata`, `SourceTraceID`.

There is no public bulk insert endpoint for dataset items — `POST /api/public/dataset-items` is single-item only and the SDKs offer no batch alternative. Items are upserted individually per trace. The server supplies a deterministic dataset item `id` derived from dataset name and source trace ID, so a stale local checkpoint or manual rerun updates the same Langfuse dataset item instead of creating a duplicate.

---

### Dataset Provisioning

Datasets are created eagerly when a deployment goes active. `DeployWorker` fires a `provisionDataset` goroutine after marking the deployment active. The goroutine fetches Langfuse credentials for the account, calls `CreateDataset` in Langfuse, and inserts the `deployment_datasets` row. This is best-effort — errors are logged but do not fail the deploy job.

Both `DeployWorker.provisionDataset` and `DatasetSyncWorker` share an `ensureDataset` helper that handles the create-if-not-exists logic idempotently. The sync worker's `ensureDataset` call acts as a fallback for deployments that were active before this feature was introduced.

---

### Daily Sync Job

Two job types handle dataset sync.

```
DatasetSyncSchedulerArgs (daily)
  query all active deployments
  insert DatasetSyncArgs per deployment
        ↓
DatasetSyncArgs (per deployment, fanned out)
        ↓
resolve Langfuse credentials
  └── not configured → skip
        ↓
ensureDataset (create if not exists)
        ↓
GetTraces [last_trace_at → now]
  └── no new traces → continue to finalization
        ↓
Upsert dataset item per trace (HTTP API)
        ↓
update last_trace_at = MAX(processed trace.createdAt)
        ↓
finalize sync attempt
  └── records last_sync_attempted_at always
  └── records last_synced_at only when the job completes successfully
```

**`DatasetSyncSchedulerArgs{}`** is registered in `periodic.go` at a `24h` interval with `RunOnStart: false`, guarded by `cfg.LangfuseStore != nil`. It queries all active deployments and inserts one `DatasetSyncArgs{DeploymentID: ...}` per deployment.

**`DatasetSyncArgs{DeploymentID string}`** runs one sync per deployment. Uses `UniqueOpts{ByArgs: true, ByState: [available, pending, retryable, running, scheduled]}` — terminal states (`completed`, `discarded`, `cancelled`) are excluded so a new job can be inserted after the previous one finishes.

Dataset sync fetches trace pages with `fields=core,io` and `orderBy=timestamp.asc` so Langfuse does not hydrate scores, metrics, or observation trees, and so offset-based pagination across the frozen `[fromTimestamp, toTimestamp]` window stays stable across requests. Traces with null input are treated as processed and skipped before any dataset item write. Each dataset item carries deterministic `id = hash(datasetName, trace.ID)`, `input = trace.Input`, `expectedOutput = trace.Output` (historical actual — regression baseline), `sourceTraceID = trace.ID`, and `metadata = trace.Metadata`.

The local dataset row is a checkpoint and summary for the Astro API. Once the dataset row exists, sync finalization is attempted even if a later trace-page read fails: the worker refreshes the Langfuse item count when useful, updates the local checkpoint when it has a processed trace timestamp, and marks the sync attempt. Checkpoint update failures are logged but do not change Langfuse idempotency: a later sync with the same trace writes the same deterministic Langfuse dataset item ID and repairs the summary without creating duplicate dataset items. Dataset item writes are best-effort: transient failures are logged for retry, while permanent validation failures advance the processed marker so one invalid trace cannot block future syncs.

---

### API

#### `GET /api/v1/deployments/:id/dataset`

Returns dataset summary metadata from the local DB. Returns 404 if the dataset row does not yet exist.

```json
{
  "dataset_name": "dep-dep_abc123",
  "last_trace_at": "2026-05-26T23:59:59Z",
  "last_sync_attempted_at": "2026-05-27T00:01:00Z",
  "last_synced_at": "2026-05-27T00:01:00Z",
  "item_count": 42
}
```

#### `POST /api/v1/deployments/:id/dataset/sync`

Enqueues an immediate `dataset.sync` job for the deployment. Returns `202 Accepted`. Respects River's unique constraint — if a sync job is already queued or running, the request is a no-op.

#### `GET /api/v1/deployments/:id/dataset/download`

Streams a zip archive containing a single JSONL file (`dep-{id}.jsonl`), with one JSON object per line. Langfuse pages are fetched and written into the zip entry incrementally — no full buffering. If Langfuse fails mid-stream, the connection closes and the client's download manager reports a failed download. Response headers: `Content-Type: application/zip`, `Content-Disposition: attachment; filename="dep-{id}.zip"`.

---

### UI

#### Tab

`{ label: "Eval", path: "dataset", icon: FlaskConical }` added to the `TABS` array in `AgentTabBar.tsx`, between Deployments and Configure. The path stays `dataset` to match the server route.

#### Eval page (`AgentDataset.tsx`)

Shows dataset name, item count, and last synced time from the `GET /dataset` endpoint. When no dataset row exists the page shows an inline unavailable message (no card wrapper).

Header buttons (shown only when data is present):
- **Sync** — calls `POST /dataset/sync`; shows a checkmark + "Scheduled" for 1 s after success, then reverts.
- **Download** — `<a href="/dataset/download" download>` link.
