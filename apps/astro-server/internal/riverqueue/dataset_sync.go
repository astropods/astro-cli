package riverqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/datasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// DatasetSyncSchedulerArgs fans out one DatasetSyncArgs per active deployment.
type DatasetSyncSchedulerArgs struct{}

func (DatasetSyncSchedulerArgs) Kind() string { return "dataset.sync_scheduler" }

func init() {
	registerJobKind[DatasetSyncSchedulerArgs]()
	registerJobKind[DatasetSyncArgs]()
}

func (DatasetSyncSchedulerArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 2}
}

// DatasetSyncSchedulerWorker queries all active deployments and enqueues one
// DatasetSyncArgs job per deployment.
type DatasetSyncSchedulerWorker struct {
	river.WorkerDefaults[DatasetSyncSchedulerArgs]
	deploymentStore *deploymentstore.Store
	queue           *Queue
	log             *logger.Logger
}

func (w *DatasetSyncSchedulerWorker) Work(ctx context.Context, _ *river.Job[DatasetSyncSchedulerArgs]) error {
	deployments, err := w.deploymentStore.ListAllActive()
	if err != nil {
		w.log.Error("Dataset sync scheduler: list active deployments", "error", err)
		return fmt.Errorf("list active deployments: %w", err)
	}

	var enqueued int
	for _, dep := range deployments {
		if _, err := w.queue.Insert(ctx, DatasetSyncArgs{DeploymentID: dep.ID}, nil); err != nil {
			w.log.Warn("Dataset sync scheduler: enqueue job", "deployment_id", dep.ID, "error", err)
			continue
		}
		enqueued++
	}

	w.log.Info("Dataset sync scheduler: enqueued jobs", "enqueued", enqueued, "total", len(deployments))
	return nil
}

// DatasetSyncArgs are the arguments for the per-deployment dataset sync job.
type DatasetSyncArgs struct {
	DeploymentID string `json:"deployment_id"`
}

func (DatasetSyncArgs) Kind() string { return "dataset.sync" }

func (DatasetSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// DatasetSyncWorker syncs one deployment's traces into a Langfuse dataset.
type DatasetSyncWorker struct {
	river.WorkerDefaults[DatasetSyncArgs]
	deploymentStore *deploymentstore.Store
	datasetStore    *datasetstore.Store
	langfuseStore   *langfuse.Store
	langfuseBaseURL string
	log             *logger.Logger
}

func (w *DatasetSyncWorker) Work(ctx context.Context, job *river.Job[DatasetSyncArgs]) error {
	if w.langfuseStore == nil {
		w.log.Warn("Dataset sync: langfuse not configured, skipping", "deployment_id", job.Args.DeploymentID)
		return nil
	}
	dep, err := w.deploymentStore.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		w.log.Warn("Dataset sync: deployment not found", "deployment_id", job.Args.DeploymentID)
		return nil
	}

	creds, err := w.langfuseStore.Get(dep.AccountID)
	if err != nil {
		return fmt.Errorf("get langfuse creds: %w", err)
	}
	if creds == nil {
		return nil
	}

	client := langfuse.NewClient(w.langfuseBaseURL, creds.PublicKey, creds.SecretKey)

	dataset, err := ensureDataset(ctx, dep, w.datasetStore, client)
	if err != nil {
		return fmt.Errorf("ensure dataset: %w", err)
	}

	var fromTimestamp string
	if dataset.LastTraceAt != nil {
		fromTimestamp = dataset.LastTraceAt.UTC().Format(time.RFC3339)
	}
	toTimestamp := time.Now().UTC().Format(time.RFC3339)

	const pageSize = 50
	const maxPages = 200
	var maxTraceAt time.Time
	var newItems int

	for page := 1; page <= maxPages; page++ {
		resp, err := client.GetTraces(ctx, dep.ID, fromTimestamp, toTimestamp, pageSize, (page-1)*pageSize)
		if err != nil {
			return fmt.Errorf("get traces page %d: %w", page, err)
		}
		if len(resp.Data) == 0 {
			break
		}

		for _, trace := range resp.Data {
			rootObsID, obsErr := client.GetRootObservationID(ctx, trace.ID)
			if obsErr != nil {
				w.log.Warn("Dataset sync: get root observation failed", "trace_id", trace.ID, "error", obsErr)
			}

			item := langfuse.DatasetItemInput{
				DatasetName:         dataset.LangfuseDatasetName,
				Input:               trace.Input,
				ExpectedOutput:      trace.Output,
				Metadata:            trace.Metadata,
				SourceTraceID:       trace.ID,
				SourceObservationID: rootObsID,
			}
			if err := client.UpsertDatasetItem(ctx, item); err != nil {
				w.log.Warn("Dataset sync: upsert item failed", "trace_id", trace.ID, "error", err)
				continue
			}
			newItems++

			if t, err := time.Parse(time.RFC3339, trace.CreatedAt); err == nil && t.After(maxTraceAt) {
				maxTraceAt = t
			}
		}

		if page >= resp.Meta.TotalPages || resp.Meta.TotalPages == 0 {
			break
		}
	}

	if newItems > 0 && !maxTraceAt.IsZero() {
		// Fetch the authoritative total from Langfuse so item_count reflects
		// reality even when re-syncing traces that were already upserted.
		countResp, countErr := client.GetDatasetItems(ctx, dataset.LangfuseDatasetName, 1, 1)
		if countErr != nil {
			w.log.Warn("Dataset sync: get total item count failed", "deployment_id", dep.ID, "error", countErr)
		} else if err := w.datasetStore.UpdateLastTraceAt(dep.ID, maxTraceAt, countResp.Meta.TotalItems); err != nil {
			w.log.Warn("Dataset sync: update last_trace_at failed", "deployment_id", dep.ID, "error", err)
		}
	}
	if err := w.datasetStore.MarkSynced(dep.ID); err != nil {
		w.log.Warn("Dataset sync: mark synced failed", "deployment_id", dep.ID, "error", err)
	}

	return nil
}

// ensureDataset returns the existing eval_datasets row, creating it in
// both Langfuse and the DB if it does not yet exist.
func ensureDataset(ctx context.Context, dep *deploymentstore.Deployment, dsStore *datasetstore.Store, client *langfuse.Client) (*datasetstore.EvalDataset, error) {
	existing, err := dsStore.Get(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("get dataset row: %w", err)
	}
	if existing != nil {
		return existing, nil
	}
	datasetName := "dep-" + dep.ID
	if err := client.CreateDataset(ctx, datasetName, dep.AgentName); err != nil {
		return nil, fmt.Errorf("create langfuse dataset: %w", err)
	}
	record := &datasetstore.EvalDataset{
		DeploymentID:        dep.ID,
		AccountID:           dep.AccountID,
		LangfuseDatasetName: datasetName,
	}
	if err := dsStore.Create(record); err != nil {
		return nil, fmt.Errorf("create dataset row: %w", err)
	}
	canonical, err := dsStore.Get(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("re-read dataset row: %w", err)
	}
	return canonical, nil
}
