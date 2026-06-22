package handlers

import (
	"archive/zip"
	"crypto/sha256"
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
	DatasetName   string  `json:"dataset_name"`
	ItemCount     int     `json:"item_count"`
	GoodCount     int     `json:"good_count"`
	BadCount      int     `json:"bad_count"`
	Grade         string  `json:"grade"`
	GradeProgress float64 `json:"grade_progress"`
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
	return evalDatasetSummary{
		DatasetName: ds.LangfuseDatasetName,
		ItemCount:   ds.Total(),
		GoodCount:   ds.GoodCount,
		BadCount:    ds.BadCount,
		Grade:       evaldataset.Grade(ds.GoodCount, ds.BadCount),
		GradeProgress: evaldataset.GradeProgress(
			ds.GoodCount,
			ds.BadCount,
		),
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
			datasetItemID = hashID(ds.LangfuseDatasetName, body.TraceID)

			if err := lctx.Client.UpsertDatasetItem(c.Request.Context(), langfuse.DatasetItemInput{
				ID:             datasetItemID,
				DatasetName:    ds.LangfuseDatasetName,
				Input:          trace.Input,
				ExpectedOutput: trace.Output,
				SourceTraceID:  body.TraceID,
				Metadata: map[string]any{
					"verdict":           effect.langfuseScore,
					"confidence":        100,
					"judged_by_user_id": lctx.UserID,
					"judged_at":         time.Now().UTC().Format(time.RFC3339),
				},
			}); err != nil {
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
