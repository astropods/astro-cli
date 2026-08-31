package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldismissalstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type DatasetReviewQueueRun struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Error  *string `json:"error"`
}

// DatasetReviewQueueItem is one trace awaiting dataset review.
type DatasetReviewQueueItem struct {
	TraceID   string                 `json:"trace_id"`
	Timestamp string                 `json:"timestamp"`
	Input     any                    `json:"input"`
	Run       *DatasetReviewQueueRun `json:"run"`
}

// DatasetReviewQueueResponse is one cursor-paginated review queue page.
type DatasetReviewQueueResponse struct {
	Items      []DatasetReviewQueueItem `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

type reviewQueueEvaluationFilter string

type reviewQueueScanStore interface {
	AddedTraceIDs(context.Context, string, []string) (map[string]bool, error)
}

type reviewQueueDismissalStore interface {
	DismissedTraceIDs(context.Context, string, []string) (map[string]bool, error)
}

type reviewQueueRunStore interface {
	LatestRuns(context.Context, string, []string) (map[string]evalrunstore.Run, error)
	TracesWithCompletedRuns(
		ctx context.Context,
		evalDatasetID string,
		startTime, endTime time.Time,
		before *evalrunstore.RunTrace,
		limit int,
	) ([]evalrunstore.RunTrace, error)
}

const (
	reviewQueueDefaultLimit  = 50
	reviewQueueMaxLimit      = 100
	reviewQueueMaxScanPages  = 3
	reviewQueueWindow        = 30 * 24 * time.Hour
	reviewQueueCursorVersion = 1
	reviewQueueEvaluated     = reviewQueueEvaluationFilter("evaluated")
	reviewQueueNotEvaluated  = reviewQueueEvaluationFilter("not_evaluated")
)

var (
	errInvalidReviewQueueCursor = errors.New("invalid review queue cursor")
	errReviewQueueLocalRead     = errors.New("review queue local read")
)

type reviewQueueCursor struct {
	Version       int    `json:"v"`
	EvalDatasetID string `json:"dataset"`
	Filter        string `json:"evaluation"`
	Limit         int    `json:"limit"`
	EndTime       string `json:"end_time"`
	RawPage       int    `json:"raw_page"`
	RawIndex      int    `json:"raw_index"`
	LocalTime     string `json:"local_time,omitempty"`
	LocalTrace    string `json:"local_trace,omitempty"`
}

// GetDatasetReviewQueue returns one cursor-paginated batch of traces that are
// neither dataset items nor dismissed, preserving Langfuse's newest-first ordering.
// GET /api/v1/deployments/:id/dataset/review-queue?limit=&evaluation=&cursor=
func GetDatasetReviewQueue(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	itemStore *evalitemstore.Store,
	runStore *evalrunstore.Store,
	dismissalStore *evaldismissalstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		evaluationFilter, ok := parseReviewQueueEvaluationFilter(c.Query("evaluation"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "evaluation must be evaluated or not_evaluated"})
			return
		}
		limit := reviewQueueDefaultLimit
		if raw := c.Query("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= reviewQueueMaxLimit {
				limit = n
			}
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		resp, err := getDatasetReviewQueuePage(
			c.Request.Context(),
			lctx.Client,
			itemStore,
			runStore,
			dismissalStore,
			ds.ID,
			lctx.DeploymentID,
			limit,
			evaluationFilter,
			strings.TrimSpace(c.Query("cursor")),
		)
		if err != nil {
			if errors.Is(err, errInvalidReviewQueueCursor) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
			if errors.Is(err, errReviewQueueLocalRead) {
				log.Error("dataset review queue: load review queue state failed", "error", err, "deployment_id", lctx.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review queue state"})
				return
			}
			log.Error("dataset review queue: fetch traces for queue failed", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch traces"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func parseReviewQueueEvaluationFilter(raw string) (reviewQueueEvaluationFilter, bool) {
	switch filter := reviewQueueEvaluationFilter(strings.ToLower(strings.TrimSpace(raw))); filter {
	case "":
		return "", true
	case reviewQueueEvaluated, reviewQueueNotEvaluated:
		return filter, true
	default:
		return "", false
	}
}

func getDatasetReviewQueuePage(
	ctx context.Context,
	client *langfuse.Client,
	itemStore reviewQueueScanStore,
	runStore reviewQueueRunStore,
	dismissalStore reviewQueueDismissalStore,
	evalDatasetID, deploymentID string,
	limit int,
	evaluationFilter reviewQueueEvaluationFilter,
	cursor string,
) (DatasetReviewQueueResponse, error) {
	scanCursor := newReviewQueueCursor(evalDatasetID, evaluationFilter, limit)
	if cursor != "" {
		decoded, err := decodeReviewQueueCursor(cursor, evalDatasetID, evaluationFilter, limit)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
		scanCursor = decoded
	}

	if evaluationFilter == reviewQueueEvaluated {
		return getEvaluatedReviewQueuePage(
			ctx,
			client,
			itemStore,
			runStore,
			dismissalStore,
			evalDatasetID,
			deploymentID,
			limit,
			scanCursor,
		)
	}
	return scanLangfuseReviewQueuePages(
		ctx,
		client,
		itemStore,
		runStore,
		dismissalStore,
		evalDatasetID,
		deploymentID,
		limit,
		evaluationFilter,
		scanCursor,
	)
}

func newReviewQueueCursor(
	evalDatasetID string,
	evaluationFilter reviewQueueEvaluationFilter,
	limit int,
) reviewQueueCursor {
	return reviewQueueCursor{
		Version:       reviewQueueCursorVersion,
		EvalDatasetID: evalDatasetID,
		Filter:        string(evaluationFilter),
		Limit:         limit,
		EndTime:       time.Now().UTC().Format(time.RFC3339Nano),
		RawPage:       1,
	}
}

// getEvaluatedReviewQueuePage pages traces with a completed run from the local
// database, then fetches only that bounded trace set from Langfuse. LocalTime
// and LocalTrace identify the last database row selected.
func getEvaluatedReviewQueuePage(
	ctx context.Context,
	client *langfuse.Client,
	itemStore reviewQueueScanStore,
	runStore reviewQueueRunStore,
	dismissalStore reviewQueueDismissalStore,
	evalDatasetID, deploymentID string,
	limit int,
	cursor reviewQueueCursor,
) (DatasetReviewQueueResponse, error) {
	asOf, err := time.Parse(time.RFC3339Nano, cursor.EndTime)
	if err != nil {
		return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
	}
	startTime := asOf.Add(-reviewQueueWindow)
	var before *evalrunstore.RunTrace
	if cursor.LocalTrace != "" {
		timestamp, err := time.Parse(time.RFC3339Nano, cursor.LocalTime)
		if err != nil {
			return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
		}
		before = &evalrunstore.RunTrace{
			TraceID:        cursor.LocalTrace,
			TraceTimestamp: timestamp,
		}
	}
	matchingTraces, err := runStore.TracesWithCompletedRuns(
		ctx,
		evalDatasetID,
		startTime,
		asOf,
		before,
		limit+1,
	)
	if err != nil {
		return DatasetReviewQueueResponse{}, fmt.Errorf("%w: completed runs: %w", errReviewQueueLocalRead, err)
	}
	if len(matchingTraces) == 0 {
		return DatasetReviewQueueResponse{Items: []DatasetReviewQueueItem{}}, nil
	}
	hasMore := len(matchingTraces) > limit
	if hasMore {
		matchingTraces = matchingTraces[:limit]
	}
	matchingTraceIDs := make([]string, 0, len(matchingTraces))
	for _, trace := range matchingTraces {
		matchingTraceIDs = append(matchingTraceIDs, trace.TraceID)
	}

	traces, err := client.GetTracesFilteredOrdered(
		ctx,
		deploymentID,
		startTime.Format(time.RFC3339Nano),
		cursor.EndTime,
		[]langfuse.TraceFilter{{
			Type:     "stringOptions",
			Column:   "id",
			Operator: "any of",
			Value:    matchingTraceIDs,
		}},
		"core,io",
		len(matchingTraceIDs),
		0,
		"timestamp.desc",
	)
	if err != nil {
		return DatasetReviewQueueResponse{}, err
	}
	items := []DatasetReviewQueueItem{}
	if len(traces.Data) > 0 {
		items, _, _, err = loadReviewQueueItems(
			ctx,
			itemStore,
			runStore,
			dismissalStore,
			evalDatasetID,
			traces.Data,
			0,
			limit,
			reviewQueueEvaluated,
		)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
	}
	if !hasMore {
		return DatasetReviewQueueResponse{Items: items}, nil
	}
	last := matchingTraces[len(matchingTraces)-1]
	cursor.LocalTime = last.TraceTimestamp.UTC().Format(time.RFC3339Nano)
	cursor.LocalTrace = last.TraceID
	nextCursor, err := encodeReviewQueueCursor(cursor)
	if err != nil {
		return DatasetReviewQueueResponse{}, fmt.Errorf("%w: encode cursor: %w", errReviewQueueLocalRead, err)
	}
	return DatasetReviewQueueResponse{Items: items, NextCursor: nextCursor}, nil
}

// scanLangfuseReviewQueuePages fills an unfiltered or no-prediction page by
// scanning recent Langfuse traces and applying local eligibility checks. Since
// skipped traces do not count toward the response limit, RawPage and RawIndex
// preserve the exact Langfuse position at which the next request should resume.
func scanLangfuseReviewQueuePages(
	ctx context.Context,
	client *langfuse.Client,
	itemStore reviewQueueScanStore,
	runStore reviewQueueRunStore,
	dismissalStore reviewQueueDismissalStore,
	evalDatasetID, deploymentID string,
	limit int,
	evaluationFilter reviewQueueEvaluationFilter,
	cursor reviewQueueCursor,
) (DatasetReviewQueueResponse, error) {
	out := make([]DatasetReviewQueueItem, 0, limit)
	endTime, err := time.Parse(time.RFC3339Nano, cursor.EndTime)
	if err != nil {
		return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
	}
	startTime := endTime.Add(-reviewQueueWindow).Format(time.RFC3339Nano)
	for rawPage, scannedPages := cursor.RawPage, 0; scannedPages < reviewQueueMaxScanPages; rawPage, scannedPages = rawPage+1, scannedPages+1 {
		traces, err := client.GetQueueTraces(
			ctx,
			deploymentID,
			startTime,
			cursor.EndTime,
			reviewQueueMaxLimit,
			(rawPage-1)*reviewQueueMaxLimit,
		)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
		if len(traces.Data) == 0 {
			return DatasetReviewQueueResponse{Items: out}, nil
		}

		startIndex := 0
		if rawPage == cursor.RawPage {
			startIndex = cursor.RawIndex
		}
		items, nextIndex, full, err := loadReviewQueueItems(
			ctx,
			itemStore,
			runStore,
			dismissalStore,
			evalDatasetID,
			traces.Data,
			startIndex,
			limit-len(out),
			evaluationFilter,
		)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
		out = append(out, items...)
		if full {
			return reviewQueuePageResponse(cursor, rawPage, traces, out, nextIndex)
		}
		if !reviewQueueHasNextRawPage(traces, rawPage) {
			return DatasetReviewQueueResponse{Items: out}, nil
		}
	}

	cursor.RawPage += reviewQueueMaxScanPages
	cursor.RawIndex = 0
	nextCursor, err := encodeReviewQueueCursor(cursor)
	if err != nil {
		return DatasetReviewQueueResponse{}, fmt.Errorf("%w: encode cursor: %w", errReviewQueueLocalRead, err)
	}
	return DatasetReviewQueueResponse{Items: out, NextCursor: nextCursor}, nil
}

func reviewQueuePageResponse(
	cursor reviewQueueCursor,
	rawPage int,
	traces *langfuse.TracesResponse,
	items []DatasetReviewQueueItem,
	nextIndex int,
) (DatasetReviewQueueResponse, error) {
	if nextIndex < len(traces.Data) {
		cursor.RawPage = rawPage
		cursor.RawIndex = nextIndex
	} else if reviewQueueHasNextRawPage(traces, rawPage) {
		cursor.RawPage = rawPage + 1
		cursor.RawIndex = 0
	} else {
		return DatasetReviewQueueResponse{Items: items}, nil
	}
	nextCursor, err := encodeReviewQueueCursor(cursor)
	if err != nil {
		return DatasetReviewQueueResponse{}, fmt.Errorf("%w: encode cursor: %w", errReviewQueueLocalRead, err)
	}
	return DatasetReviewQueueResponse{Items: items, NextCursor: nextCursor}, nil
}

// loadReviewQueueItems batch-loads the state each evaluation filter needs and
// builds up to limit queue items.
func loadReviewQueueItems(
	ctx context.Context,
	itemStore reviewQueueScanStore,
	runStore reviewQueueRunStore,
	dismissalStore reviewQueueDismissalStore,
	evalDatasetID string,
	traces []langfuse.Trace,
	startIndex, limit int,
	evaluationFilter reviewQueueEvaluationFilter,
) ([]DatasetReviewQueueItem, int, bool, error) {
	traceIDs := make([]string, 0, len(traces))
	for _, trace := range traces {
		traceIDs = append(traceIDs, trace.ID)
	}
	added, err := itemStore.AddedTraceIDs(ctx, evalDatasetID, traceIDs)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%w: dataset items: %w", errReviewQueueLocalRead, err)
	}
	dismissed, err := dismissalStore.DismissedTraceIDs(ctx, evalDatasetID, traceIDs)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%w: dismissals: %w", errReviewQueueLocalRead, err)
	}
	runs, err := runStore.LatestRuns(ctx, evalDatasetID, traceIDs)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%w: runs: %w", errReviewQueueLocalRead, err)
	}

	items := make([]DatasetReviewQueueItem, 0, limit)
	for i := startIndex; i < len(traces); i++ {
		trace := traces[i]
		if added[trace.ID] || dismissed[trace.ID] || trace.Input == nil {
			continue
		}
		run, hasRun := runs[trace.ID]
		evaluated := hasRun && run.Status == evalrunstore.StatusCompleted
		if evaluationFilter == reviewQueueNotEvaluated && evaluated {
			continue
		}
		if evaluationFilter == reviewQueueEvaluated && !evaluated {
			continue
		}
		items = append(items, newDatasetReviewQueueItem(trace, run, hasRun))
		if len(items) == limit {
			return items, i + 1, true, nil
		}
	}
	return items, len(traces), false, nil
}

func reviewQueueHasNextRawPage(traces *langfuse.TracesResponse, rawPage int) bool {
	if len(traces.Data) == 0 {
		return false
	}
	if traces.Meta.TotalPages > 0 {
		page := traces.Meta.Page
		if page <= 0 {
			page = rawPage
		}
		return page < traces.Meta.TotalPages
	}
	if traces.Meta.TotalItems > 0 {
		return rawPage*reviewQueueMaxLimit < traces.Meta.TotalItems
	}
	return len(traces.Data) == reviewQueueMaxLimit
}

func encodeReviewQueueCursor(cursor reviewQueueCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode review queue cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeReviewQueueCursor(
	raw, evalDatasetID string,
	evaluationFilter reviewQueueEvaluationFilter,
	limit int,
) (reviewQueueCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return reviewQueueCursor{}, fmt.Errorf("%w: decode", errInvalidReviewQueueCursor)
	}
	var cursor reviewQueueCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return reviewQueueCursor{}, fmt.Errorf("%w: unmarshal", errInvalidReviewQueueCursor)
	}
	if cursor.Version != reviewQueueCursorVersion ||
		cursor.EvalDatasetID != evalDatasetID ||
		cursor.Filter != string(evaluationFilter) ||
		cursor.Limit != limit {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.EndTime); err != nil {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if evaluationFilter == reviewQueueEvaluated {
		if cursor.LocalTrace == "" {
			return reviewQueueCursor{}, errInvalidReviewQueueCursor
		}
		if _, err := time.Parse(time.RFC3339Nano, cursor.LocalTime); err != nil {
			return reviewQueueCursor{}, errInvalidReviewQueueCursor
		}
		return cursor, nil
	}
	if cursor.RawPage < 1 ||
		cursor.RawIndex < 0 ||
		cursor.RawIndex >= reviewQueueMaxLimit {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	return cursor, nil
}

func newDatasetReviewQueueItem(
	trace langfuse.Trace,
	run evalrunstore.Run,
	hasRun bool,
) DatasetReviewQueueItem {
	item := DatasetReviewQueueItem{
		TraceID:   trace.ID,
		Timestamp: trace.Timestamp,
		Input:     trace.Input,
	}
	if !hasRun {
		return item
	}
	queueRun := DatasetReviewQueueRun{ID: run.ID, Status: string(run.Status)}
	if run.ErrorMessage != "" {
		message := run.ErrorMessage
		queueRun.Error = &message
	}
	item.Run = &queueRun
	return item
}
