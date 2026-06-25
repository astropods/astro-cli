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
	"sort"
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
)

// evalDatasetSummary is the JSON shape returned by GetEvalDataset.
type evalDatasetSummary struct {
	DatasetName       string  `json:"dataset_name"`
	ItemCount         int     `json:"item_count"`
	GoodCount         int     `json:"good_count"`
	BadCount          int     `json:"bad_count"`
	Grade             string  `json:"grade"`
	NextGrade         string  `json:"next_grade"`
	NextGradeProgress float64 `json:"next_grade_progress"`
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

func summaryFromRow(ds *evaldatasetstore.EvalDataset) evalDatasetSummary {
	nextGrade, nextGradeProgress := evaldataset.NextGradeProgress(ds.GoodCount, ds.BadCount)
	return evalDatasetSummary{
		DatasetName:       ds.LangfuseDatasetName,
		ItemCount:         ds.Total(),
		GoodCount:         ds.GoodCount,
		BadCount:          ds.BadCount,
		Grade:             evaldataset.Grade(ds.GoodCount, ds.BadCount),
		NextGrade:         nextGrade,
		NextGradeProgress: nextGradeProgress,
	}
}

// GetEvalDataset returns dataset summary metadata from the local DB.
// GET /api/v1/deployments/:id/dataset
func GetEvalDataset(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
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

		c.JSON(http.StatusOK, summaryFromRow(ds))
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

// datasetReviewQueueItem is the shape returned to the client for one trace awaiting dataset review.
type datasetReviewQueueItem struct {
	TraceID   string `json:"trace_id"`
	Timestamp string `json:"timestamp"`
	Input     any    `json:"input"`
	Output    any    `json:"output"`
	Sentiment string `json:"sentiment"`
}

type datasetReviewQueueResponse struct {
	Items      []datasetReviewQueueItem `json:"items"`
	NextOffset int                      `json:"next_offset,omitempty"`
	EndTime    string                   `json:"end_time"`
}

const (
	reviewQueueDefaultLimit = 50
	reviewQueueMaxLimit     = 100
)

// GetDatasetReviewQueue returns one paginated batch of traces awaiting dataset
// judgment, sentiment-tagged then sorted (sentiment first, then recency desc).
// end_time is a validated RFC3339 snapshot token. Clients must echo it unchanged
// when requesting later offsets so pagination stays within one trace window.
// GET /api/v1/deployments/:id/dataset/review-queue?offset=&limit=&end_time=
func GetDatasetReviewQueue(
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

		limit := reviewQueueDefaultLimit
		if raw := c.Query("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= reviewQueueMaxLimit {
				limit = n
			}
		}
		offset := 0
		if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
				return
			}
			offset = n
		}
		if offset%limit != 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a multiple of limit"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}
		endTime, ok := reviewQueueEndTime(c, offset)
		if !ok {
			return
		}

		traces, err := lctx.Client.GetQueueTraces(c.Request.Context(), lctx.DeploymentID, endTime, limit, offset)
		if err != nil {
			log.Error("Failed to fetch traces for queue", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch traces"})
			return
		}

		traceIDs := make([]string, 0, len(traces.Data))
		for _, t := range traces.Data {
			traceIDs = append(traceIDs, t.ID)
		}

		judged, err := judgmentStore.JudgedTraceIDs(ds.ID, traceIDs)
		if err != nil {
			log.Error("Failed to load judged ids", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to filter queue"})
			return
		}

		annotated := annotateQueue(traces.Data, judged)

		resp := datasetReviewQueueResponse{Items: annotated, EndTime: endTime}
		resp.NextOffset = nextReviewQueueOffset(len(traces.Data), traces.Meta.TotalItems, offset)
		c.JSON(http.StatusOK, resp)
	}
}

func reviewQueueEndTime(c *gin.Context, offset int) (string, bool) {
	raw := strings.TrimSpace(c.Query("end_time"))
	if raw == "" {
		if offset > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end_time is required when offset is non-zero"})
			return "", false
		}
		return time.Now().UTC().Format(time.RFC3339Nano), true
	}
	if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time"})
		return "", false
	}
	return raw, true
}

func nextReviewQueueOffset(pageSize, totalItems, offset int) int {
	if pageSize == 0 {
		return 0
	}
	nextOffset := offset + pageSize
	if totalItems <= nextOffset {
		return 0
	}
	return nextOffset
}

// annotateQueue groups the page by session, sorts each session ascending,
// infers sentiment from the next trace's input, filters out already-judged
// traces, then returns the survivors sorted sentiment-first then recency desc.
func annotateQueue(traces []langfuse.Trace, judged map[string]bool) []datasetReviewQueueItem {
	bySession := map[string][]langfuse.Trace{}
	for _, t := range traces {
		key := t.SessionID
		if key == "" {
			key = "__none__:" + t.ID
		}
		bySession[key] = append(bySession[key], t)
	}

	sentiment := map[string]evaldataset.Sentiment{}
	for _, list := range bySession {
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt < list[j].CreatedAt })
		for i, t := range list {
			if i+1 < len(list) {
				sentiment[t.ID] = evaldataset.InferFromAny(list[i+1].Input)
			}
		}
	}

	items := make([]datasetReviewQueueItem, 0, len(traces))
	for _, t := range traces {
		if judged[t.ID] || t.Input == nil {
			continue
		}
		items = append(items, datasetReviewQueueItem{
			TraceID:   t.ID,
			Timestamp: t.CreatedAt,
			Input:     t.Input,
			Output:    t.Output,
			Sentiment: string(sentiment[t.ID]),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		ai := items[i].Sentiment != ""
		aj := items[j].Sentiment != ""
		if ai != aj {
			return ai
		}
		return items[i].Timestamp > items[j].Timestamp
	})

	return items
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
	NextCursor string               `json:"next_cursor,omitempty"`
}

const (
	itemsDefaultLimit = 50
	itemsMaxLimit     = 100
	itemsScanSlack    = 2
)

type evalDatasetItemsVerdict string

const (
	evalDatasetItemsVerdictGood evalDatasetItemsVerdict = "good"
	evalDatasetItemsVerdictBad  evalDatasetItemsVerdict = "bad"
)

const evalDatasetItemsCursorVersion = 1

var errInvalidEvalDatasetItemsCursor = errors.New("invalid dataset items cursor")

type evalDatasetItemsCursor struct {
	Version     int    `json:"v"`
	DatasetName string `json:"dataset"`
	Verdict     string `json:"verdict"`
	Limit       int    `json:"limit"`
	RawPage     int    `json:"raw_page"`
	RawIndex    int    `json:"raw_index"`
	Matched     int    `json:"matched"`
}

// GetEvalDatasetItems returns a page of judged items from the Langfuse dataset.
// GET /api/v1/deployments/:id/dataset/items?page=&limit=&verdict=&cursor=
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
		verdict, ok := parseEvalDatasetItemsVerdict(c.Query("verdict"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "verdict must be good or bad"})
			return
		}
		cursor := strings.TrimSpace(c.Query("cursor"))
		if verdict == "" && cursor != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cursor requires verdict"})
			return
		}
		if verdict != "" && cursor != "" && strings.TrimSpace(c.Query("page")) != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "page cannot be used with cursor"})
			return
		}
		if verdict != "" && cursor == "" && strings.TrimSpace(c.Query("page")) != "" && page != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "filtered dataset items use cursor pagination"})
			return
		}

		resp, nextCursor, err := getEvalDatasetItemsPage(c.Request.Context(), lctx.Client, ds, page, limit, verdict, cursor)
		if err != nil {
			if errors.Is(err, errInvalidEvalDatasetItemsCursor) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
				return
			}
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
			NextCursor: nextCursor,
		})
	}
}

func parseEvalDatasetItemsVerdict(raw string) (evalDatasetItemsVerdict, bool) {
	switch evalDatasetItemsVerdict(strings.ToLower(strings.TrimSpace(raw))) {
	case "":
		return "", true
	case evalDatasetItemsVerdictGood:
		return evalDatasetItemsVerdictGood, true
	case evalDatasetItemsVerdictBad:
		return evalDatasetItemsVerdictBad, true
	default:
		return "", false
	}
}

func getEvalDatasetItemsPage(
	ctx context.Context,
	client *langfuse.Client,
	ds *evaldatasetstore.EvalDataset,
	page int,
	limit int,
	verdict evalDatasetItemsVerdict,
	cursor string,
) (*langfuse.DatasetItemsResponse, string, error) {
	if verdict == "" {
		resp, err := client.GetDatasetItems(ctx, ds.LangfuseDatasetName, page, limit)
		return resp, "", err
	}

	totalItems := ds.GoodCount
	if verdict == evalDatasetItemsVerdictBad {
		totalItems = ds.BadCount
	}

	scanCursor := evalDatasetItemsCursor{
		Version:     evalDatasetItemsCursorVersion,
		DatasetName: ds.LangfuseDatasetName,
		Verdict:     string(verdict),
		Limit:       limit,
		RawPage:     1,
		RawIndex:    0,
	}
	if cursor != "" {
		decoded, err := decodeEvalDatasetItemsCursor(cursor, ds.LangfuseDatasetName, verdict, limit)
		if err != nil {
			return nil, "", err
		}
		scanCursor = decoded
	}
	page = (scanCursor.Matched / limit) + 1

	// Langfuse does not currently support filtering dataset items by metadata.
	// While eval datasets are small, scan Langfuse pages here so the UI can
	// offer real Good/Bad pagination. If datasets grow large, replace this with
	// a local index or a native Langfuse metadata filter.
	if scanCursor.Matched >= totalItems {
		return filteredDatasetItemsResponse(nil, page, limit, totalItems), "", nil
	}

	matched := scanCursor.Matched
	out := make([]langfuse.DatasetItem, 0, limit)
	maxScanPages := maxDatasetItemsScanPages(ds, totalItems)

	for upstreamPage, scannedPages := scanCursor.RawPage, 0; scannedPages < maxScanPages; upstreamPage, scannedPages = upstreamPage+1, scannedPages+1 {
		resp, err := client.GetDatasetItems(ctx, ds.LangfuseDatasetName, upstreamPage, itemsMaxLimit)
		if err != nil {
			return nil, "", err
		}
		startIndex := 0
		if upstreamPage == scanCursor.RawPage {
			startIndex = scanCursor.RawIndex
		}
		for i := startIndex; i < len(resp.Data); i++ {
			item := resp.Data[i]
			if !datasetItemMatchesVerdict(item, verdict) {
				continue
			}
			out = append(out, item)
			matched++
			if len(out) >= limit || matched >= totalItems {
				nextCursor := ""
				if matched < totalItems {
					rawPage, rawIndex := nextDatasetItemsCursorPosition(upstreamPage, i+1, len(resp.Data))
					nextCursor, err = encodeEvalDatasetItemsCursor(evalDatasetItemsCursor{
						Version:     evalDatasetItemsCursorVersion,
						DatasetName: ds.LangfuseDatasetName,
						Verdict:     string(verdict),
						Limit:       limit,
						RawPage:     rawPage,
						RawIndex:    rawIndex,
						Matched:     matched,
					})
					if err != nil {
						return nil, "", err
					}
				}
				return filteredDatasetItemsResponse(out, page, limit, totalItems), nextCursor, nil
			}
		}
		if len(resp.Data) == 0 || (resp.Meta.TotalPages > 0 && resp.Meta.Page >= resp.Meta.TotalPages) {
			return exhaustedFilteredDatasetItemsResponse(out, page, limit, totalItems, matched), "", nil
		}
	}
	return exhaustedFilteredDatasetItemsResponse(out, page, limit, totalItems, matched), "", nil
}

func exhaustedFilteredDatasetItemsResponse(items []langfuse.DatasetItem, page, limit, totalItems, matched int) *langfuse.DatasetItemsResponse {
	if matched < totalItems {
		totalItems = matched
	}
	return filteredDatasetItemsResponse(items, page, limit, totalItems)
}

func maxDatasetItemsScanPages(ds *evaldatasetstore.EvalDataset, filteredTotal int) int {
	datasetSize := ds.Total()
	if filteredTotal > datasetSize {
		datasetSize = filteredTotal
	}
	pages := (datasetSize + itemsMaxLimit - 1) / itemsMaxLimit
	if pages < 1 {
		pages = 1
	}
	return pages + itemsScanSlack
}

func nextDatasetItemsCursorPosition(rawPage, nextIndex, pageSize int) (int, int) {
	if nextIndex >= pageSize {
		return rawPage + 1, 0
	}
	return rawPage, nextIndex
}

func encodeEvalDatasetItemsCursor(cursor evalDatasetItemsCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode dataset items cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeEvalDatasetItemsCursor(raw, datasetName string, verdict evalDatasetItemsVerdict, limit int) (evalDatasetItemsCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return evalDatasetItemsCursor{}, fmt.Errorf("%w: decode", errInvalidEvalDatasetItemsCursor)
	}
	var cursor evalDatasetItemsCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return evalDatasetItemsCursor{}, fmt.Errorf("%w: unmarshal", errInvalidEvalDatasetItemsCursor)
	}
	if cursor.Version != evalDatasetItemsCursorVersion ||
		cursor.DatasetName != datasetName ||
		cursor.Verdict != string(verdict) ||
		cursor.Limit != limit ||
		cursor.RawPage < 1 ||
		cursor.RawIndex < 0 ||
		cursor.RawIndex >= itemsMaxLimit ||
		cursor.Matched < 0 {
		return evalDatasetItemsCursor{}, errInvalidEvalDatasetItemsCursor
	}
	return cursor, nil
}

func filteredDatasetItemsResponse(items []langfuse.DatasetItem, page, limit, totalItems int) *langfuse.DatasetItemsResponse {
	resp := &langfuse.DatasetItemsResponse{Data: items}
	resp.Meta.Page = page
	resp.Meta.Limit = limit
	resp.Meta.TotalItems = totalItems
	resp.Meta.TotalPages = totalPages(totalItems, limit)
	return resp
}

func totalPages(totalItems, limit int) int {
	if totalItems == 0 {
		return 0
	}
	return (totalItems + limit - 1) / limit
}

func datasetItemMatchesVerdict(item langfuse.DatasetItem, verdict evalDatasetItemsVerdict) bool {
	metadata, ok := item.Metadata.(map[string]any)
	if !ok {
		return false
	}
	switch v := metadata["verdict"].(type) {
	case float64:
		if verdict == evalDatasetItemsVerdictGood {
			return v == 1
		}
		return v == -1
	case string:
		return strings.EqualFold(v, string(verdict))
	default:
		return false
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
	langfuseScore    int
	goodDelta        int
	badDelta         int
}

func effectForVerdict(v judgmentstore.Verdict) judgmentEffect {
	switch v {
	case judgmentstore.VerdictGood:
		return judgmentEffect{writeDatasetItem: true, langfuseScore: 1, goodDelta: 1}
	case judgmentstore.VerdictBad:
		return judgmentEffect{writeDatasetItem: true, langfuseScore: -1, badDelta: 1}
	default:
		return judgmentEffect{}
	}
}

func reverseJudgmentEffect(effect judgmentEffect) judgmentEffect {
	return judgmentEffect{
		writeDatasetItem: effect.writeDatasetItem,
		langfuseScore:    -effect.langfuseScore,
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
			"verdict":           effect.langfuseScore,
			"confidence":        100,
			"judged_by_user_id": lctx.UserID,
			"judged_at":         time.Now().UTC().Format(time.RFC3339),
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

		if !traceHasDeploymentTag(trace.Tags, lctx.DeploymentID) {
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
			datasetItemID, err = upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, body.TraceID, effect)
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

		if !traceHasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "trace does not belong to this deployment"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		previous, found, err := judgmentStore.UpdateReturningPrevious(ds.ID, traceID, verdict)
		if err != nil {
			log.Error("Failed to update dataset judgment", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update judgment"})
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "judgment not found"})
			return
		}

		restoreJudgment := func(reason string) {
			if _, _, err := judgmentStore.UpdateReturningPrevious(ds.ID, traceID, previous); err != nil {
				log.Warn("Failed to restore judgment row after verdict change failure", "error", err, "trace_id", traceID, "reason", reason)
			}
		}

		previousEffect := effectForVerdict(previous)
		nextEffect := effectForVerdict(verdict)
		datasetItemID := hashID(ds.LangfuseDatasetName, traceID)

		if nextEffect.writeDatasetItem {
			if _, err := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, nextEffect); err != nil {
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
					if _, rollbackErr := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, previousEffect); rollbackErr != nil {
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
