package handlers

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// evalDatasetSummary is the JSON shape returned by GetEvalDataset.
type evalDatasetSummary struct {
	DatasetName       string                      `json:"dataset_name"`
	ItemCount         int                         `json:"item_count"`
	GoodCount         int                         `json:"good_count"`
	BadCount          int                         `json:"bad_count"`
	Grade             string                      `json:"grade"`
	NextGrade         string                      `json:"next_grade"`
	NextGradeProgress float64                     `json:"next_grade_progress"`
	CasesToNextGrade  *int                        `json:"cases_to_next_grade"`
	CriteriaCounts    []evalDatasetCriterionCount `json:"criteria_counts"`
}

type evalDatasetCriterionCount struct {
	DimensionKey string `json:"dimension_key"`
	GoodCount    int    `json:"good_count"`
	BadCount     int    `json:"bad_count"`
}

// loadDataset fetches the dataset row for a deployment and writes the matching
// error response when missing or unreadable. Returns (nil, false) if the caller
// should stop; otherwise (row, true).
func loadDataset(
	c *gin.Context,
	log *logger.Logger,
	datasetStore *evaldatasetstore.Store,
	deploymentID string,
) (*evaldatasetstore.EvalDataset, bool) {
	ds, err := datasetStore.GetByDeploymentID(deploymentID)
	if err != nil {
		log.Error("Failed to get dataset record", "error", err, "deployment_id", deploymentID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset"})
		return nil, false
	}
	if ds == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset not yet created"})
		return nil, false
	}
	return ds, true
}

func summaryFromRow(ds *evaldatasetstore.EvalDataset, criteriaCounts []judgmentstore.CriterionCounts) evalDatasetSummary {
	nextGrade, nextGradeProgress := evaldataset.NextGradeProgress(ds.GoodCount, ds.BadCount)
	return evalDatasetSummary{
		DatasetName:       ds.LangfuseDatasetName,
		ItemCount:         ds.Total(),
		GoodCount:         ds.GoodCount,
		BadCount:          ds.BadCount,
		Grade:             evaldataset.Grade(ds.GoodCount, ds.BadCount),
		NextGrade:         nextGrade,
		NextGradeProgress: nextGradeProgress,
		CasesToNextGrade:  evaldataset.CasesToNextGrade(ds.GoodCount, ds.BadCount),
		CriteriaCounts:    summaryCriterionCounts(criteriaCounts),
	}
}

func summaryCriterionCounts(counts []judgmentstore.CriterionCounts) []evalDatasetCriterionCount {
	byDimension := make(map[judgmentstore.CriterionDimension]judgmentstore.CriterionCounts, len(counts))
	for _, count := range counts {
		if count.Dimension.Valid() {
			byDimension[count.Dimension] = count
		}
	}

	out := make([]evalDatasetCriterionCount, 0, len(judgmentstore.CriterionDimensions))
	for _, dimension := range judgmentstore.CriterionDimensions {
		count := byDimension[dimension]
		out = append(out, evalDatasetCriterionCount{
			DimensionKey: string(dimension),
			GoodCount:    count.GoodCount,
			BadCount:     count.BadCount,
		})
	}
	return out
}

// GetEvalDataset returns dataset summary metadata from the local DB.
// GET /api/v1/deployments/:id/dataset
func GetEvalDataset(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	judgmentStore *judgmentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		deploymentID := c.Param("id")
		dep, err := deploymentStore.GetDeploymentByID(deploymentID)
		if err != nil || dep == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		isMember, err := accountStore.IsMember(dep.AccountID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, deploymentID)
		if !ok {
			return
		}

		criteriaCounts, err := judgmentStore.CriterionCounts(ds.ID)
		if err != nil {
			log.Error("Failed to get dataset criterion counts", "error", err, "dataset_id", ds.ID, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset criteria counts"})
			return
		}

		c.JSON(http.StatusOK, summaryFromRow(ds, criteriaCounts))
	}
}

// DownloadEvalDataset streams a zip archive containing a JSONL file with all dataset items.
// GET /api/v1/deployments/:id/dataset/download
func DownloadEvalDataset(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		zipName := ds.LangfuseDatasetName + ".zip"
		jsonlName := ds.LangfuseDatasetName + ".jsonl"

		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipName))

		zw := zip.NewWriter(c.Writer)
		defer zw.Close() //nolint:errcheck

		fw, err := zw.Create(jsonlName)
		if err != nil {
			log.Error("Failed to create zip entry", "error", err)
			return
		}

		enc := json.NewEncoder(fw)
		const pageSize = 100
		for page := 1; ; page++ {
			items, pageErr := lctx.Client.GetDatasetItems(c.Request.Context(), ds.LangfuseDatasetName, page, pageSize)
			if pageErr != nil {
				log.Error("Failed to fetch dataset items for download", "error", pageErr, "page", page, "deployment_id", lctx.DeploymentID)
				return
			}
			for _, item := range items.Data {
				row := map[string]any{
					"id":                    item.ID,
					"input":                 item.Input,
					"expected_output":       item.ExpectedOutput,
					"metadata":              item.Metadata,
					"source_trace_id":       item.SourceTraceID,
					"source_observation_id": item.SourceObservationID,
					"created_at":            item.CreatedAt,
				}
				if encErr := enc.Encode(row); encErr != nil {
					log.Error("Failed to write JSONL entry", "error", encErr)
					return
				}
			}
			if len(items.Data) == 0 || page >= items.Meta.TotalPages || items.Meta.TotalPages == 0 {
				break
			}
		}
	}
}

type DatasetReviewQueuePredictionCriterion struct {
	DimensionKey   string  `json:"dimension_key"`
	DimensionValue float64 `json:"dimension_value"`
}

type DatasetReviewQueuePrediction struct {
	VerdictScore float64                                 `json:"verdict_score"`
	Confidence   int                                     `json:"confidence"`
	Explanation  string                                  `json:"explanation"`
	JudgeVersion string                                  `json:"judge_version"`
	Criteria     []DatasetReviewQueuePredictionCriterion `json:"criteria"`
}

// DatasetReviewQueueItem is one trace awaiting dataset review.
type DatasetReviewQueueItem struct {
	TraceID          string                        `json:"trace_id"`
	Timestamp        string                        `json:"timestamp"`
	UserID           string                        `json:"user_id,omitempty"`
	UserDetails      *UserDetails                  `json:"user_details,omitempty"`
	Input            any                           `json:"input"`
	Output           any                           `json:"output"`
	PredictionStatus string                        `json:"prediction_status"`
	PredictionError  *string                       `json:"prediction_error"`
	Prediction       *DatasetReviewQueuePrediction `json:"prediction"`
}

// DatasetReviewQueueResponse is one cursor-paginated review queue page.
type DatasetReviewQueueResponse struct {
	Items      []DatasetReviewQueueItem `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

type reviewQueuePredictionFilter string

type reviewQueueScanStore interface {
	JudgedTraceIDs(context.Context, string, []string) (map[string]bool, error)
	GetPredictionRequests(context.Context, string, []string) (map[string]judgmentstore.PredictionRequest, error)
	GetPredictions(context.Context, string, []string) (map[string]judgmentstore.Prediction, error)
}

const (
	reviewQueueDefaultLimit       = 50
	reviewQueueMaxLimit           = 100
	reviewQueueMaxScanPages       = 3
	reviewQueueWindow             = 30 * 24 * time.Hour
	reviewQueueCursorVersion      = 1
	reviewQueuePredictionPresent  = reviewQueuePredictionFilter("present")
	reviewQueuePredictionAbsent   = reviewQueuePredictionFilter("absent")
	reviewQueueStatusNotRequested = "not_requested"
)

var (
	errInvalidReviewQueueCursor = errors.New("invalid review queue cursor")
	errReviewQueueLocalRead     = errors.New("review queue local read")
)

type reviewQueueCursor struct {
	Version          int    `json:"v"`
	EvalDatasetID    string `json:"dataset"`
	PredictionFilter string `json:"prediction"`
	Limit            int    `json:"limit"`
	EndTime          string `json:"end_time"`
	RawPage          int    `json:"raw_page"`
	RawIndex         int    `json:"raw_index"`
	PredictionTime   string `json:"prediction_time,omitempty"`
	PredictionTrace  string `json:"prediction_trace,omitempty"`
}

// GetDatasetReviewQueue returns one cursor-paginated batch of traces awaiting
// dataset judgment, preserving Langfuse's newest-first ordering.
// GET /api/v1/deployments/:id/dataset/review-queue?limit=&prediction=&cursor=
func GetDatasetReviewQueue(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	judgmentStore *judgmentstore.Store,
	slackStore *slackidentity.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		predictionFilter, ok := parseReviewQueuePredictionFilter(c.Query("prediction"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prediction must be present or absent"})
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
			judgmentStore,
			ds.ID,
			lctx.DeploymentID,
			limit,
			predictionFilter,
			strings.TrimSpace(c.Query("cursor")),
		)
		if err != nil {
			if errors.Is(err, errInvalidReviewQueueCursor) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
			if errors.Is(err, errReviewQueueLocalRead) {
				log.Error("Failed to load review queue state", "error", err, "deployment_id", lctx.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review queue state"})
				return
			}
			log.Error("Failed to fetch traces for queue", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch traces"})
			return
		}
		hydrateDatasetReviewQueueUsers(log, slackStore, accountStore, &resp)
		c.JSON(http.StatusOK, resp)
	}
}

func hydrateDatasetReviewQueueUsers(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	accountStore *account.AccountStore,
	resp *DatasetReviewQueueResponse,
) {
	if resp == nil || len(resp.Items) == 0 {
		return
	}
	userIDs := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.UserID != "" {
			userIDs = append(userIDs, item.UserID)
		}
	}
	if len(userIDs) == 0 {
		return
	}
	hydrator := newUserDetailsHydrator(log, slackStore, accountStore, userIDs, "dataset-review-queue")
	for i := range resp.Items {
		resp.Items[i].UserDetails = traceUserDetailsFromHydrator(resp.Items[i].UserID, hydrator)
	}
}

func parseReviewQueuePredictionFilter(raw string) (reviewQueuePredictionFilter, bool) {
	switch filter := reviewQueuePredictionFilter(strings.ToLower(strings.TrimSpace(raw))); filter {
	case "":
		return "", true
	case reviewQueuePredictionPresent, reviewQueuePredictionAbsent:
		return filter, true
	default:
		return "", false
	}
}

func getDatasetReviewQueuePage(
	ctx context.Context,
	client *langfuse.Client,
	judgmentStore *judgmentstore.Store,
	evalDatasetID, deploymentID string,
	limit int,
	predictionFilter reviewQueuePredictionFilter,
	cursor string,
) (DatasetReviewQueueResponse, error) {
	scanCursor := newReviewQueueCursor(evalDatasetID, predictionFilter, limit)
	if cursor != "" {
		decoded, err := decodeReviewQueueCursor(cursor, evalDatasetID, predictionFilter, limit)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
		scanCursor = decoded
	}

	if predictionFilter == reviewQueuePredictionPresent {
		return getPredictedReviewQueuePage(
			ctx,
			client,
			judgmentStore,
			evalDatasetID,
			deploymentID,
			limit,
			scanCursor,
		)
	}
	return scanLangfuseReviewQueuePages(
		ctx,
		client,
		judgmentStore,
		evalDatasetID,
		deploymentID,
		limit,
		predictionFilter,
		scanCursor,
	)
}

func newReviewQueueCursor(
	evalDatasetID string,
	predictionFilter reviewQueuePredictionFilter,
	limit int,
) reviewQueueCursor {
	return reviewQueueCursor{
		Version:          reviewQueueCursorVersion,
		EvalDatasetID:    evalDatasetID,
		PredictionFilter: string(predictionFilter),
		Limit:            limit,
		EndTime:          time.Now().UTC().Format(time.RFC3339Nano),
		RawPage:          1,
	}
}

// getPredictedReviewQueuePage pages prediction records without judgments from
// the local database, then fetches only that bounded trace set from Langfuse.
// PredictionTime and PredictionTrace identify the last database row selected.
func getPredictedReviewQueuePage(
	ctx context.Context,
	client *langfuse.Client,
	judgmentStore *judgmentstore.Store,
	evalDatasetID, deploymentID string,
	limit int,
	cursor reviewQueueCursor,
) (DatasetReviewQueueResponse, error) {
	asOf, err := time.Parse(time.RFC3339Nano, cursor.EndTime)
	if err != nil {
		return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
	}
	startTime := asOf.Add(-reviewQueueWindow)
	var before *judgmentstore.PredictionTrace
	if cursor.PredictionTrace != "" {
		timestamp, err := time.Parse(time.RFC3339Nano, cursor.PredictionTime)
		if err != nil {
			return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
		}
		before = &judgmentstore.PredictionTrace{
			TraceID:        cursor.PredictionTrace,
			TraceTimestamp: timestamp,
		}
	}
	matchingTraces, err := judgmentStore.PredictionTracesWithoutJudgments(
		ctx,
		evalDatasetID,
		startTime,
		asOf,
		before,
		limit+1,
	)
	if err != nil {
		return DatasetReviewQueueResponse{}, fmt.Errorf("%w: prediction traces: %w", errReviewQueueLocalRead, err)
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
			judgmentStore,
			evalDatasetID,
			traces.Data,
			0,
			limit,
			reviewQueuePredictionPresent,
		)
		if err != nil {
			return DatasetReviewQueueResponse{}, err
		}
	}
	if !hasMore {
		return DatasetReviewQueueResponse{Items: items}, nil
	}
	last := matchingTraces[len(matchingTraces)-1]
	cursor.PredictionTime = last.TraceTimestamp.UTC().Format(time.RFC3339Nano)
	cursor.PredictionTrace = last.TraceID
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
	judgmentStore reviewQueueScanStore,
	evalDatasetID, deploymentID string,
	limit int,
	predictionFilter reviewQueuePredictionFilter,
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
			judgmentStore,
			evalDatasetID,
			traces.Data,
			startIndex,
			limit-len(out),
			predictionFilter,
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

// loadReviewQueueItems batch-loads the state needed by each prediction filter
// and builds up to limit queue items. Present candidates already exclude
// judgments in SQL. All and absent candidates require the scan-path reads.
func loadReviewQueueItems(
	ctx context.Context,
	judgmentStore reviewQueueScanStore,
	evalDatasetID string,
	traces []langfuse.Trace,
	startIndex, limit int,
	predictionFilter reviewQueuePredictionFilter,
) ([]DatasetReviewQueueItem, int, bool, error) {
	traceIDs := make([]string, 0, len(traces))
	for _, trace := range traces {
		traceIDs = append(traceIDs, trace.ID)
	}
	var err error
	var judged map[string]bool
	var requests map[string]judgmentstore.PredictionRequest
	if predictionFilter != reviewQueuePredictionPresent {
		judged, err = judgmentStore.JudgedTraceIDs(ctx, evalDatasetID, traceIDs)
		if err != nil {
			return nil, 0, false, fmt.Errorf("%w: judgments: %w", errReviewQueueLocalRead, err)
		}
		requests, err = judgmentStore.GetPredictionRequests(ctx, evalDatasetID, traceIDs)
		if err != nil {
			return nil, 0, false, fmt.Errorf("%w: requests: %w", errReviewQueueLocalRead, err)
		}
	}
	predictions, err := judgmentStore.GetPredictions(ctx, evalDatasetID, traceIDs)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%w: predictions: %w", errReviewQueueLocalRead, err)
	}

	items := make([]DatasetReviewQueueItem, 0, limit)
	for i := startIndex; i < len(traces); i++ {
		trace := traces[i]
		if judged[trace.ID] || trace.Input == nil {
			continue
		}
		prediction, hasPrediction := predictions[trace.ID]
		if predictionFilter == reviewQueuePredictionAbsent && hasPrediction {
			continue
		}
		items = append(items, newDatasetReviewQueueItem(trace, requests[trace.ID], prediction, hasPrediction))
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
	predictionFilter reviewQueuePredictionFilter,
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
		cursor.PredictionFilter != string(predictionFilter) ||
		cursor.Limit != limit {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.EndTime); err != nil {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if predictionFilter == reviewQueuePredictionPresent {
		if cursor.PredictionTrace == "" {
			return reviewQueueCursor{}, errInvalidReviewQueueCursor
		}
		if _, err := time.Parse(time.RFC3339Nano, cursor.PredictionTime); err != nil {
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
	request judgmentstore.PredictionRequest,
	prediction judgmentstore.Prediction,
	hasPrediction bool,
) DatasetReviewQueueItem {
	item := DatasetReviewQueueItem{
		TraceID:          trace.ID,
		Timestamp:        trace.CreatedAt,
		UserID:           trace.UserID,
		Input:            trace.Input,
		Output:           trace.Output,
		PredictionStatus: reviewQueueStatusNotRequested,
	}
	if hasPrediction {
		criteriaByDimension := make(map[judgmentstore.CriterionDimension]float64, len(prediction.Criteria))
		for _, criterion := range prediction.Criteria {
			criteriaByDimension[criterion.Dimension] = criterion.Value
		}
		criteria := make([]DatasetReviewQueuePredictionCriterion, 0, len(prediction.Criteria))
		for _, dimension := range judgmentstore.CriterionDimensions {
			value, ok := criteriaByDimension[dimension]
			if !ok {
				continue
			}
			criteria = append(criteria, DatasetReviewQueuePredictionCriterion{
				DimensionKey:   string(dimension),
				DimensionValue: value,
			})
		}
		item.PredictionStatus = string(judgmentstore.PredictionRequestCompleted)
		item.Prediction = &DatasetReviewQueuePrediction{
			VerdictScore: prediction.VerdictScore,
			Confidence:   prediction.Confidence,
			Explanation:  prediction.Explanation,
			JudgeVersion: prediction.JudgeVersion,
			Criteria:     criteria,
		}
		return item
	}
	if request.Status != "" && request.Status != judgmentstore.PredictionRequestCompleted {
		item.PredictionStatus = string(request.Status)
		item.PredictionError = request.ErrorMessage
	}
	return item
}

// evalDatasetItemsResponse mirrors the Langfuse list response 1:1, narrowed to
// the fields the UI uses.
type evalDatasetItemRow struct {
	ID             string `json:"id"`
	Input          any    `json:"input"`
	ExpectedOutput any    `json:"expected_output"`
	Metadata       any    `json:"metadata"`
	SourceTraceID  string `json:"source_trace_id"`
	CreatedAt      string `json:"created_at"`
}

type evalDatasetItemsResponse struct {
	Items      []evalDatasetItemRow `json:"items"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	TotalItems int                  `json:"total_items"`
	TotalPages int                  `json:"total_pages"`
}

const (
	itemsDefaultLimit = 50
	itemsMaxLimit     = 100
)

// GetEvalDatasetItems returns a page of judged items from the Langfuse dataset.
// GET /api/v1/deployments/:id/dataset/items?page=&limit=
func GetEvalDatasetItems(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		page := 1
		if raw := c.Query("page"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				page = n
			}
		}
		limit := itemsDefaultLimit
		if raw := c.Query("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= itemsMaxLimit {
				limit = n
			}
		}
		resp, err := lctx.Client.GetDatasetItems(c.Request.Context(), ds.LangfuseDatasetName, page, limit)
		if err != nil {
			log.Error("Failed to list dataset items", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch dataset items"})
			return
		}

		rows := make([]evalDatasetItemRow, 0, len(resp.Data))
		for _, item := range resp.Data {
			rows = append(rows, evalDatasetItemRow{
				ID:             item.ID,
				Input:          item.Input,
				ExpectedOutput: item.ExpectedOutput,
				Metadata:       item.Metadata,
				SourceTraceID:  item.SourceTraceID,
				CreatedAt:      item.CreatedAt,
			})
		}

		c.JSON(http.StatusOK, evalDatasetItemsResponse{
			Items:      rows,
			Page:       resp.Meta.Page,
			Limit:      resp.Meta.Limit,
			TotalItems: resp.Meta.TotalItems,
			TotalPages: resp.Meta.TotalPages,
		})
	}
}

type DatasetJudgmentRequest struct {
	TraceID string `json:"trace_id"`
	Verdict string `json:"verdict"`
}

type DatasetJudgmentResponse struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
	Verdict       string `json:"verdict"`
}

type judgmentEffect struct {
	writeDatasetItem bool
	goodDelta        int
	badDelta         int
}

func effectForVerdict(v judgmentstore.Verdict) judgmentEffect {
	switch v {
	case judgmentstore.VerdictGood:
		return judgmentEffect{writeDatasetItem: true, goodDelta: 1}
	case judgmentstore.VerdictBad:
		return judgmentEffect{writeDatasetItem: true, badDelta: 1}
	default:
		return judgmentEffect{}
	}
}

func reverseJudgmentEffect(effect judgmentEffect) judgmentEffect {
	return judgmentEffect{
		writeDatasetItem: effect.writeDatasetItem,
		goodDelta:        -effect.goodDelta,
		badDelta:         -effect.badDelta,
	}
}

func upsertJudgmentDatasetItem(
	ctx context.Context,
	lctx *langfuseContext,
	ds *evaldatasetstore.EvalDataset,
	trace *langfuse.TraceDetail,
	traceID string,
	effect judgmentEffect,
	criteria []judgmentstore.Reason,
) (string, error) {
	if !effect.writeDatasetItem {
		return "", nil
	}

	datasetItemID := hashID(ds.LangfuseDatasetName, traceID)
	if err := lctx.Client.UpsertDatasetItem(ctx, langfuse.DatasetItemInput{
		ID:             datasetItemID,
		DatasetName:    ds.LangfuseDatasetName,
		Input:          trace.Input,
		ExpectedOutput: trace.Output,
		SourceTraceID:  traceID,
		Metadata: map[string]any{
			"judged_by_user_id": lctx.UserID,
			"judged_at":         time.Now().UTC().Format(time.RFC3339),
			"judgment_criteria": reasonsToCriteria(criteria),
		},
	}); err != nil {
		return "", err
	}

	return datasetItemID, nil
}

// PostDatasetJudgment records a verdict for a trace and, for good/bad, writes the
// corresponding Langfuse dataset item and bumps the local counters.
// POST /api/v1/deployments/:id/dataset/judgments
func PostDatasetJudgment(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	judgmentStore *judgmentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		var body DatasetJudgmentRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		body.TraceID = strings.TrimSpace(body.TraceID)
		if body.TraceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
			return
		}
		verdict := judgmentstore.Verdict(strings.ToLower(strings.TrimSpace(body.Verdict)))
		if !verdict.Valid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid verdict %q", body.Verdict)})
			return
		}

		trace, err := lctx.Client.GetTrace(c.Request.Context(), body.TraceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("Failed to fetch trace for judgment", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch trace"})
			return
		}

		if !langfuse.HasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "trace does not belong to this deployment"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		if ds.LangfuseDatasetName != evaldataset.ExpectedName(lctx.DeploymentID) {
			ensured, err := evaldataset.Ensure(c.Request.Context(), datasetStore, lctx.Client, evaldataset.EnsureOptions{
				DeploymentID: lctx.DeploymentID,
			})
			if err != nil {
				log.Error("Failed to ensure eval dataset", "error", err, "deployment_id", lctx.DeploymentID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare dataset"})
				return
			}
			ds = ensured
		}

		// Insert before any mutating upstream write as the duplicate gate. Any
		// retry or double-click now loses before it can upsert the Langfuse item.
		if err := judgmentStore.Insert(ds.ID, body.TraceID, verdict); err != nil {
			if errors.Is(err, judgmentstore.ErrAlreadyJudged) {
				c.JSON(http.StatusConflict, gin.H{"error": "trace already judged"})
				return
			}
			log.Error("Failed to record judgment", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record judgment"})
			return
		}
		rollbackJudgment := func(reason string) {
			if err := judgmentStore.Delete(ds.ID, body.TraceID); err != nil {
				log.Warn("Failed to roll back judgment row", "error", err, "trace_id", body.TraceID, "reason", reason)
			}
		}

		effect := effectForVerdict(verdict)
		var datasetItemID string
		if effect.writeDatasetItem {
			var err error
			datasetItemID, err = upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, body.TraceID, effect, nil)
			if err != nil {
				rollbackJudgment("dataset item write failed")
				log.Error("Failed to upsert dataset item", "error", err, "trace_id", body.TraceID)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
				return
			}
		}

		if effect.goodDelta != 0 || effect.badDelta != 0 {
			if err := datasetStore.BumpCountsByID(ds.ID, effect.goodDelta, effect.badDelta); err != nil {
				if datasetItemID != "" {
					// Keep the local judgment row in place until after Langfuse
					// compensation so a retry cannot recreate the item before this
					// request deletes it.
					if deleteErr := lctx.Client.DeleteDatasetItem(c.Request.Context(), datasetItemID); deleteErr != nil && !errors.Is(deleteErr, langfuse.ErrNotFound) {
						log.Warn("Failed to roll back Langfuse dataset item", "error", deleteErr, "trace_id", body.TraceID, "dataset_item_id", datasetItemID)
					}
				}
				rollbackJudgment("dataset count bump failed")
				log.Error("Failed to bump dataset counts", "error", err, "deployment_id", lctx.DeploymentID,
					"good_delta", effect.goodDelta, "bad_delta", effect.badDelta)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update dataset counts"})
				return
			}
		}

		c.JSON(http.StatusCreated, DatasetJudgmentResponse{
			EvalDatasetID: ds.ID,
			TraceID:       body.TraceID,
			Verdict:       string(verdict),
		})
	}
}

// PatchDatasetJudgment changes an existing judged trace's verdict without
// returning the trace to the review queue.
// PATCH /api/v1/deployments/:id/dataset/judgments/:trace_id
func PatchDatasetJudgment(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	judgmentStore *judgmentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traceID := strings.TrimSpace(c.Param("trace_id"))
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
			return
		}

		var body DatasetJudgmentRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		verdict := judgmentstore.Verdict(strings.ToLower(strings.TrimSpace(body.Verdict)))
		if !verdict.Valid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid verdict %q", body.Verdict)})
			return
		}

		trace, err := lctx.Client.GetTrace(c.Request.Context(), traceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("Failed to fetch trace for judgment change", "error", err, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch trace"})
			return
		}

		if !langfuse.HasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "trace does not belong to this deployment"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		previous, previousReasons, found, err := judgmentStore.SetVerdictAndReasons(ds.ID, traceID, verdict, nil)
		if err != nil {
			log.Error("Failed to update dataset judgment", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update judgment"})
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "judgment not found"})
			return
		}

		if previous == verdict {
			c.JSON(http.StatusOK, DatasetJudgmentResponse{
				EvalDatasetID: ds.ID,
				TraceID:       traceID,
				Verdict:       string(verdict),
			})
			return
		}

		restoreJudgment := func(reason string) {
			if _, _, _, err := judgmentStore.SetVerdictAndReasons(ds.ID, traceID, previous, previousReasons); err != nil {
				log.Warn("Failed to restore judgment after verdict change failure", "error", err, "trace_id", traceID, "reason", reason)
			}
		}

		previousEffect := effectForVerdict(previous)
		nextEffect := effectForVerdict(verdict)
		datasetItemID := hashID(ds.LangfuseDatasetName, traceID)

		if nextEffect.writeDatasetItem {
			if _, err := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, nextEffect, nil); err != nil {
				restoreJudgment("dataset item upsert failed")
				log.Error("Failed to upsert changed dataset item", "error", err, "trace_id", traceID)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
				return
			}
		} else if previousEffect.writeDatasetItem {
			if err := lctx.Client.DeleteDatasetItem(c.Request.Context(), datasetItemID); err != nil && !errors.Is(err, langfuse.ErrNotFound) {
				restoreJudgment("dataset item delete failed")
				log.Error("Failed to delete changed dataset item", "error", err, "trace_id", traceID, "dataset_item_id", datasetItemID)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete dataset item"})
				return
			}
		}

		goodDelta := nextEffect.goodDelta - previousEffect.goodDelta
		badDelta := nextEffect.badDelta - previousEffect.badDelta
		if goodDelta != 0 || badDelta != 0 {
			if err := datasetStore.BumpCountsByID(ds.ID, goodDelta, badDelta); err != nil {
				if previousEffect.writeDatasetItem {
					if _, rollbackErr := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, previousEffect, previousReasons); rollbackErr != nil {
						log.Warn("Failed to restore dataset item after verdict count failure", "error", rollbackErr, "trace_id", traceID)
					}
				} else if nextEffect.writeDatasetItem {
					if deleteErr := lctx.Client.DeleteDatasetItem(c.Request.Context(), datasetItemID); deleteErr != nil && !errors.Is(deleteErr, langfuse.ErrNotFound) {
						log.Warn("Failed to delete dataset item after verdict count failure", "error", deleteErr, "trace_id", traceID)
					}
				}
				restoreJudgment("dataset count update failed")
				log.Error("Failed to update dataset counts for verdict change", "error", err, "deployment_id", lctx.DeploymentID,
					"good_delta", goodDelta, "bad_delta", badDelta)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update dataset counts"})
				return
			}
		}

		c.JSON(http.StatusOK, DatasetJudgmentResponse{
			EvalDatasetID: ds.ID,
			TraceID:       traceID,
			Verdict:       string(verdict),
		})
	}
}

type judgmentCriterion struct {
	DimensionKey string  `json:"dimension_key"`
	Value        float64 `json:"value"`
}

// judgmentCriterionInput is the request shape; Value is a pointer so an omitted
// value is rejected rather than silently binding to 0 (a valid in-range score).
type judgmentCriterionInput struct {
	DimensionKey string   `json:"dimension_key"`
	Value        *float64 `json:"value"`
}

type DatasetJudgmentCriteriaRequest struct {
	Criteria []judgmentCriterionInput `json:"criteria"`
}

func reasonsToCriteria(reasons []judgmentstore.Reason) []judgmentCriterion {
	out := make([]judgmentCriterion, len(reasons))
	for i, r := range reasons {
		out[i] = judgmentCriterion{DimensionKey: string(r.Dimension), Value: r.Value}
	}
	return out
}

type DatasetJudgmentCriteriaResponse struct {
	EvalDatasetID string              `json:"eval_dataset_id"`
	TraceID       string              `json:"trace_id"`
	Verdict       string              `json:"verdict"`
	Criteria      []judgmentCriterion `json:"criteria"`
}

// PutDatasetJudgmentCriteria replaces the selected criteria (reasons) for an
// existing good/bad judgment and updates the Langfuse dataset item metadata.
// PUT /api/v1/deployments/:id/dataset/judgments/:trace_id/criteria
func PutDatasetJudgmentCriteria(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	judgmentStore *judgmentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traceID := strings.TrimSpace(c.Param("trace_id"))
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
			return
		}

		var body DatasetJudgmentCriteriaRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		reasons := make([]judgmentstore.Reason, len(body.Criteria))
		seen := make(map[judgmentstore.CriterionDimension]bool, len(body.Criteria))
		for i, crit := range body.Criteria {
			d := judgmentstore.CriterionDimension(strings.TrimSpace(crit.DimensionKey))
			if !d.Valid() {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid criterion %q", crit.DimensionKey)})
				return
			}
			if seen[d] {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("duplicate criterion %q", d)})
				return
			}
			seen[d] = true
			if crit.Value == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("criterion %q requires a value", crit.DimensionKey)})
				return
			}
			if *crit.Value < -1 || *crit.Value > 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("criterion %q value %v out of range [-1, 1]", crit.DimensionKey, *crit.Value)})
				return
			}
			reasons[i] = judgmentstore.Reason{Dimension: d, Value: *crit.Value}
		}

		trace, err := lctx.Client.GetTrace(c.Request.Context(), traceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("Failed to fetch trace for criteria update", "error", err, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch trace"})
			return
		}
		if !langfuse.HasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "trace does not belong to this deployment"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		verdict, previous, found, err := judgmentStore.ReplaceReasons(ds.ID, traceID, reasons)
		if err != nil {
			log.Error("Failed to replace judgment criteria", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update criteria"})
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "judgment not found"})
			return
		}
		if verdict == judgmentstore.VerdictUnknown {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot set criteria on an unknown judgment"})
			return
		}

		effect := effectForVerdict(verdict)
		if _, err := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, effect, reasons); err != nil {
			if _, _, _, restoreErr := judgmentStore.ReplaceReasons(ds.ID, traceID, previous); restoreErr != nil {
				log.Warn("Failed to restore criteria after dataset item upsert failure", "error", restoreErr, "trace_id", traceID)
			}
			log.Error("Failed to upsert dataset item for criteria", "error", err, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
			return
		}

		c.JSON(http.StatusOK, DatasetJudgmentCriteriaResponse{
			EvalDatasetID: ds.ID,
			TraceID:       traceID,
			Verdict:       string(verdict),
			Criteria:      reasonsToCriteria(reasons),
		})
	}
}

// DeleteDatasetJudgment removes a prior verdict so its trace can re-enter the
// review queue. Good/bad judgments also remove the deterministic Langfuse
// dataset item and decrement the local grade counts.
// DELETE /api/v1/deployments/:id/dataset/judgments/:trace_id
func DeleteDatasetJudgment(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	judgmentStore *judgmentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traceID := strings.TrimSpace(c.Param("trace_id"))
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		verdict, found, err := judgmentStore.DeleteReturningVerdict(ds.ID, traceID)
		if err != nil {
			log.Error("Failed to remove dataset judgment", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove judgment"})
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "judgment not found"})
			return
		}

		restoreJudgment := func(reason string) {
			if err := judgmentStore.Insert(ds.ID, traceID, verdict); err != nil && !errors.Is(err, judgmentstore.ErrAlreadyJudged) {
				log.Warn("Failed to restore judgment row after undo failure", "error", err, "trace_id", traceID, "reason", reason)
			}
		}

		effect := reverseJudgmentEffect(effectForVerdict(verdict))
		if effect.goodDelta != 0 || effect.badDelta != 0 {
			if err := datasetStore.BumpCountsByID(ds.ID, effect.goodDelta, effect.badDelta); err != nil {
				restoreJudgment("dataset count decrement failed")
				log.Error("Failed to decrement dataset counts", "error", err, "deployment_id", lctx.DeploymentID,
					"good_delta", effect.goodDelta, "bad_delta", effect.badDelta)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update dataset counts"})
				return
			}
		}

		if effect.writeDatasetItem {
			datasetItemID := hashID(ds.LangfuseDatasetName, traceID)
			if err := lctx.Client.DeleteDatasetItem(c.Request.Context(), datasetItemID); err != nil && !errors.Is(err, langfuse.ErrNotFound) {
				if effect.goodDelta != 0 || effect.badDelta != 0 {
					if bumpErr := datasetStore.BumpCountsByID(ds.ID, -effect.goodDelta, -effect.badDelta); bumpErr != nil {
						log.Warn("Failed to restore dataset counts after undo failure", "error", bumpErr, "trace_id", traceID)
					}
				}
				restoreJudgment("dataset item delete failed")
				log.Error("Failed to delete dataset item", "error", err, "trace_id", traceID, "dataset_item_id", datasetItemID)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete dataset item"})
				return
			}
		}

		c.JSON(http.StatusOK, DatasetJudgmentResponse{
			EvalDatasetID: ds.ID,
			TraceID:       traceID,
			Verdict:       string(verdict),
		})
	}
}

func hashID(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
