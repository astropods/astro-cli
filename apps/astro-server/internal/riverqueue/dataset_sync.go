package riverqueue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
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

func (w *DatasetSyncWorker) Work(ctx context.Context, job *river.Job[DatasetSyncArgs]) (workErr error) {
	log := w.log.WithFields(map[string]interface{}{
		"deployment_id": job.Args.DeploymentID,
		"job_id":        job.ID,
		"attempt":       job.Attempt,
		"max_attempts":  job.MaxAttempts,
	})
	log.Info("Dataset sync: starting")

	if w.langfuseStore == nil {
		log.Warn("Dataset sync: langfuse not configured, skipping")
		log.Info("Dataset sync: finished")
		return nil
	}
	dep, err := w.deploymentStore.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil {
		log.Warn("Dataset sync: deployment not found")
		log.Info("Dataset sync: finished")
		return nil
	}

	creds, err := w.langfuseStore.Get(dep.AccountID)
	if err != nil {
		return fmt.Errorf("get langfuse creds: %w", err)
	}
	if creds == nil {
		log.Info("Dataset sync: langfuse credentials missing, skipping")
		log.Info("Dataset sync: finished")
		return nil
	}

	client := langfuse.NewClient(w.langfuseBaseURL, creds.PublicKey, creds.SecretKey)

	dataset, err := ensureDataset(ctx, dep, w.datasetStore, client)
	if err != nil {
		log.Warn("Dataset sync: ensure dataset failed", "error", err)
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
	var tracesProcessed int
	var itemsWriteAttempted int
	var itemsUpserted int
	var tracesSkipped int
	itemCount := dataset.ItemCount
	lastTraceAt := ""

	defer func() {
		finalizeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		datasetSummaryMissing := dataset.ItemCount == 0 || dataset.LastSyncAttemptedAt == nil
		shouldRefreshCount := itemsWriteAttempted > 0 || datasetSummaryMissing
		var itemCountToPersist *int
		if shouldRefreshCount {
			countResp, countErr := client.GetDatasetItems(finalizeCtx, dataset.LangfuseDatasetName, 1, 1)
			if countErr != nil {
				log.Warn("Dataset sync: get total item count failed", "error", countErr)
			} else {
				itemCount = countResp.Meta.TotalItems
				itemCountToPersist = &itemCount
			}
		}

		var lastTraceAtToPersist *time.Time
		if !maxTraceAt.IsZero() {
			traceAt := maxTraceAt.UTC()
			lastTraceAt = traceAt.Format(time.RFC3339)
			lastTraceAtToPersist = &traceAt
		}
		if err := w.datasetStore.FinalizeSync(dep.ID, itemCountToPersist, lastTraceAtToPersist, workErr == nil); err != nil {
			log.Warn("Dataset sync: finalize sync failed", "error", err)
		}

		logArgs := []interface{}{
			"traces_processed", tracesProcessed,
			"items_upserted", itemsUpserted,
			"traces_skipped", tracesSkipped,
			"item_count", itemCount,
			"last_trace_at", lastTraceAt,
		}
		if workErr != nil {
			logArgs = append(logArgs, "error", workErr)
		}
		log.Info("Dataset sync: finished", logArgs...)
	}()

	for page := 1; page <= maxPages; page++ {
		resp, err := client.GetTraces(ctx, dep.ID, fromTimestamp, toTimestamp, pageSize, (page-1)*pageSize)
		if err != nil {
			log.Warn("Dataset sync: get traces page failed",
				"page", page,
				"page_size", pageSize,
				"from_timestamp", fromTimestamp,
				"to_timestamp", toTimestamp,
				"error", err,
			)
			return fmt.Errorf("get traces page %d: %w", page, err)
		}
		if len(resp.Data) == 0 {
			break
		}

		for _, trace := range resp.Data {
			tracesProcessed++
			traceAt, traceTimeErr := time.Parse(time.RFC3339, trace.CreatedAt)
			if traceTimeErr != nil {
				log.Warn("Dataset sync: parse trace timestamp failed", "trace_id", trace.ID, "created_at", trace.CreatedAt)
			}

			if shouldSkipDatasetTrace(trace) {
				tracesSkipped++
				advanceMaxTraceAt(&maxTraceAt, traceAt, traceTimeErr)
				continue
			}

			rootObsID, obsErr := client.GetRootObservationID(ctx, trace.ID)
			if obsErr != nil {
				log.Warn("Dataset sync: get root observation failed", "trace_id", trace.ID, "error", obsErr)
			}

			item := langfuse.DatasetItemInput{
				ID:                  deterministicDatasetItemID(dataset.LangfuseDatasetName, trace.ID),
				DatasetName:         dataset.LangfuseDatasetName,
				Input:               trace.Input,
				ExpectedOutput:      trace.Output,
				Metadata:            trace.Metadata,
				SourceTraceID:       trace.ID,
				SourceObservationID: rootObsID,
			}
			itemsWriteAttempted++
			if err := client.UpsertDatasetItem(ctx, item); err != nil {
				log.Warn("Dataset sync: upsert item failed", "trace_id", trace.ID, "error", err)
				if isPermanentDatasetItemError(err) {
					advanceMaxTraceAt(&maxTraceAt, traceAt, traceTimeErr)
				}
				continue
			}
			itemsUpserted++
			advanceMaxTraceAt(&maxTraceAt, traceAt, traceTimeErr)
		}

		if page >= resp.Meta.TotalPages || resp.Meta.TotalPages == 0 {
			break
		}
	}
	return nil
}

func shouldSkipDatasetTrace(trace langfuse.Trace) bool {
	return trace.Input == nil
}

func isPermanentDatasetItemError(err error) bool {
	var apiErr *langfuse.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func advanceMaxTraceAt(maxTraceAt *time.Time, traceAt time.Time, traceTimeErr error) {
	if traceTimeErr == nil && traceAt.After(*maxTraceAt) {
		*maxTraceAt = traceAt
	}
}

func deterministicDatasetItemID(datasetName, sourceTraceID string) string {
	sum := sha256.Sum256([]byte(datasetName + "\x00" + sourceTraceID))
	return "astro-" + hex.EncodeToString(sum[:])
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
