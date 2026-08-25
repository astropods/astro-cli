package handlers

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

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
			log.Error("dataset items: create zip entry failed", "error", err)
			return
		}

		enc := json.NewEncoder(fw)
		const pageSize = 100
		for page := 1; ; page++ {
			items, pageErr := lctx.Client.GetDatasetItems(c.Request.Context(), ds.LangfuseDatasetName, page, pageSize)
			if pageErr != nil {
				log.Error("dataset items: fetch dataset items for download failed", "error", pageErr, "page", page, "deployment_id", lctx.DeploymentID)
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
					log.Error("dataset items: write JSONL entry failed", "error", encErr)
					return
				}
			}
			if len(items.Data) == 0 || page >= items.Meta.TotalPages || items.Meta.TotalPages == 0 {
				break
			}
		}
	}
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
			log.Error("dataset items: list dataset items failed", "error", err, "deployment_id", lctx.DeploymentID)
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

type datasetItemEvaluatorOutput struct {
	Key   string           `json:"key"`
	Value *json.RawMessage `json:"value"`
}

type DatasetItemRequest struct {
	TraceID          string                       `json:"trace_id"`
	EvaluationRunID  string                       `json:"evaluation_run_id"`
	EvaluatorOutputs []datasetItemEvaluatorOutput `json:"evaluator_outputs"`
}

type DatasetItemResponse struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
	EvaluationRef string `json:"evaluation_ref"`
}

// PostDatasetItem adds a trace to the dataset with a verified value for every
// evaluator in the agent's evaluation set.
// POST /api/v1/deployments/:id/dataset/items
func PostDatasetItem(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	itemStore *evalitemstore.Store,
	runStore *evalrunstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		var body DatasetItemRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		body.TraceID = strings.TrimSpace(body.TraceID)
		if body.TraceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id is required"})
			return
		}

		set, err := evalpreset.ResolveSet(evalpreset.RefDefaultSet)
		if err != nil {
			log.Error("dataset items: resolve evaluation set failed", "error", err,
				"deployment_id", lctx.DeploymentID, "evaluation_ref", evalpreset.RefDefaultSet)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return
		}
		outputs, invalid := resolveItemOutputs(set, body.EvaluatorOutputs)
		if invalid != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": invalid})
			return
		}

		trace, err := lctx.Client.GetTrace(c.Request.Context(), body.TraceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("dataset items: fetch trace failed", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch trace"})
			return
		}
		if !langfuse.HasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "trace does not belong to this deployment"})
			return
		}

		ds, ok := loadDatasetEnsured(c, log, datasetStore, lctx)
		if !ok {
			return
		}

		sourceRunID, ok := resolveItemSourceRun(c, log, runStore, ds.ID, body)
		if !ok {
			return
		}

		item := evalitemstore.Item{
			EvalDatasetID:         ds.ID,
			TraceID:               body.TraceID,
			EvaluationRef:         evalpreset.RefDefaultSet,
			SourceEvaluationRunID: sourceRunID,
			AddedByUserID:         lctx.UserID,
		}
		// Insert before the Langfuse write as the duplicate gate, so a retry or a
		// double-click loses here rather than upserting the item a second time.
		if err := itemStore.Add(c.Request.Context(), item, outputs); err != nil {
			if errors.Is(err, evalitemstore.ErrAlreadyAdded) {
				c.JSON(http.StatusConflict, gin.H{"error": "trace already in the dataset"})
				return
			}
			log.Error("dataset items: add dataset item failed", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add the dataset item"})
			return
		}

		if _, err := evaldataset.UpsertItem(c.Request.Context(), lctx.Client, evaldataset.ItemInput{
			DatasetName:    ds.LangfuseDatasetName,
			TraceID:        body.TraceID,
			Input:          trace.Input,
			ExpectedOutput: trace.Output,
		}); err != nil {
			if deleteErr := itemStore.Delete(c.Request.Context(), ds.ID, body.TraceID); deleteErr != nil {
				log.Warn("dataset items: roll back dataset item failed", "error", deleteErr, "trace_id", body.TraceID)
			}
			log.Error("dataset items: upsert Langfuse dataset item failed", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
			return
		}

		c.JSON(http.StatusCreated, DatasetItemResponse{
			EvalDatasetID: ds.ID,
			TraceID:       body.TraceID,
			EvaluationRef: evalpreset.RefDefaultSet,
		})
	}
}

func resolveItemSourceRun(
	c *gin.Context,
	log *logger.Logger,
	runStore *evalrunstore.Store,
	evalDatasetID string,
	body DatasetItemRequest,
) (*string, bool) {
	runID := strings.TrimSpace(body.EvaluationRunID)
	if runID == "" {
		return nil, true
	}
	run, err := runStore.GetRun(c.Request.Context(), runID)
	if err != nil {
		log.Error("dataset items: load evaluation run failed", "error", err, "evaluation_run_id", runID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the evaluation run"})
		return nil, false
	}
	if run == nil || run.EvalDatasetID != evalDatasetID || run.TraceID != body.TraceID ||
		run.EvaluationRef != evalpreset.RefDefaultSet {
		c.JSON(http.StatusConflict, gin.H{"error": "evaluation run does not match this trace"})
		return nil, false
	}
	return &run.ID, true
}

// resolveItemOutputs pairs each evaluator in the set with its submitted value,
// returning the outputs in definition order or the reason the request failed.
func resolveItemOutputs(
	set []evaluator.Evaluator,
	submitted []datasetItemEvaluatorOutput,
) ([]evalitemstore.Output, string) {
	byKey := make(map[string]*json.RawMessage, len(submitted))
	for _, output := range submitted {
		key := strings.TrimSpace(output.Key)
		if _, seen := byKey[key]; seen {
			return nil, fmt.Sprintf("duplicate evaluator %q", key)
		}
		byKey[key] = output.Value
	}

	outputs := make([]evalitemstore.Output, 0, len(set))
	for _, definition := range set {
		raw, submitted := byKey[definition.Key]
		if !submitted || raw == nil {
			return nil, fmt.Sprintf("evaluator %q requires a value", definition.Key)
		}
		delete(byKey, definition.Key)
		if _, err := evaluator.ValidateValue(definition.Output, *raw); err != nil {
			return nil, fmt.Sprintf("evaluator %q: %v", definition.Key, err)
		}
		outputs = append(outputs, evalitemstore.Output{EvaluatorKey: definition.Key, Value: *raw})
	}
	for key := range byKey {
		return nil, fmt.Sprintf("unknown evaluator %q", key)
	}
	return outputs, ""
}
