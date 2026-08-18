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
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
)

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
	Version       int    `json:"v"`
	EvalDatasetID string `json:"dataset"`
	Filter        string `json:"prediction"`
	Limit         int    `json:"limit"`
	EndTime       string `json:"end_time"`
	RawPage       int    `json:"raw_page"`
	RawIndex      int    `json:"raw_index"`
	LocalTime     string `json:"prediction_time,omitempty"`
	LocalTrace    string `json:"prediction_trace,omitempty"`
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
		Version:       reviewQueueCursorVersion,
		EvalDatasetID: evalDatasetID,
		Filter:        string(predictionFilter),
		Limit:         limit,
		EndTime:       time.Now().UTC().Format(time.RFC3339Nano),
		RawPage:       1,
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
	if cursor.LocalTrace != "" {
		timestamp, err := time.Parse(time.RFC3339Nano, cursor.LocalTime)
		if err != nil {
			return DatasetReviewQueueResponse{}, errInvalidReviewQueueCursor
		}
		before = &judgmentstore.PredictionTrace{
			TraceID:        cursor.LocalTrace,
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
		cursor.Filter != string(predictionFilter) ||
		cursor.Limit != limit {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.EndTime); err != nil {
		return reviewQueueCursor{}, errInvalidReviewQueueCursor
	}
	if predictionFilter == reviewQueuePredictionPresent {
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
