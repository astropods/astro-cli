package handlers

import (
	"archive/zip"
	"context"
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
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evalresolve"
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

type evalDatasetItemRow struct {
	ID               string                  `json:"id"`
	Input            any                     `json:"input"`
	ExpectedOutput   any                     `json:"expected_output"`
	SourceTraceID    string                  `json:"source_trace_id"`
	CreatedAt        string                  `json:"created_at"`
	EvaluationRef    string                  `json:"evaluation_ref,omitempty"`
	VerifiedByUserID string                  `json:"verified_by_user_id,omitempty"`
	EvaluatorOutputs []evalDatasetItemOutput `json:"evaluator_outputs"`
}

type evalDatasetItemOutput struct {
	Key   string          `json:"key"`
	Label string          `json:"label"`
	Value json.RawMessage `json:"value"`
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

// GetEvalDatasetItems returns a page of items from the Langfuse dataset, each
// carrying the evaluator outputs it was verified on.
// GET /api/v1/deployments/:id/dataset/items?page=&limit=
func GetEvalDatasetItems(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	itemStore *evalitemstore.Store,
	resolver evalSetResolver,
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

		traceIDs := make([]string, 0, len(resp.Data))
		for _, item := range resp.Data {
			traceIDs = append(traceIDs, item.SourceTraceID)
		}
		verified, err := itemStore.GetMany(c.Request.Context(), ds.ID, traceIDs)
		if err != nil {
			log.Error("dataset items: load evaluator outputs failed", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load evaluator outputs"})
			return
		}

		evaluationRef, err := resolver.ActiveRef(c.Request.Context(), lctx.AccountID, lctx.AgentName)
		if err != nil {
			log.Error("dataset items: resolve active ref failed", "error", err, "dataset_id", ds.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return
		}
		// A retired evaluator still holds values, so a set that will not resolve
		// costs labels and ordering rather than the values themselves.
		set, err := resolver.Set(c.Request.Context(), evaluationRef)
		if err != nil {
			log.Warn("dataset items: resolve evaluation set failed", "error", err,
				"dataset_id", ds.ID, "evaluation_ref", evaluationRef)
		}

		rows := make([]evalDatasetItemRow, 0, len(resp.Data))
		for _, item := range resp.Data {
			row := evalDatasetItemRow{
				ID:               item.ID,
				Input:            item.Input,
				ExpectedOutput:   item.ExpectedOutput,
				SourceTraceID:    item.SourceTraceID,
				CreatedAt:        item.CreatedAt,
				EvaluatorOutputs: []evalDatasetItemOutput{},
			}
			if local, ok := verified[item.SourceTraceID]; ok {
				row.EvaluationRef = local.EvaluationRef
				row.VerifiedByUserID = local.VerifiedByUserID
				row.EvaluatorOutputs = itemOutputs(set, local.Outputs)
			}
			rows = append(rows, row)
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

func itemOutputs(
	set []evaluator.Evaluator,
	outputs []evalitemstore.Output,
) []evalDatasetItemOutput {
	groups := evaluatorsBySet(set, outputs,
		func(output evalitemstore.Output) string { return output.EvaluatorKey })

	out := make([]evalDatasetItemOutput, 0, len(groups))
	for _, group := range groups {
		out = append(out, evalDatasetItemOutput{
			Key:   group.Key,
			Label: group.label(),
			Value: group.Rows[0].Value,
		})
	}
	return out
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

// PostDatasetItem adds a trace to the dataset with the reviewer's verified
// values, for as many evaluators in the set as they chose to record.
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
	resolver evalSetResolver,
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

		sourceRunID, evaluationRef, ok := resolveItemProvenance(c, log, runStore, resolver, ds.ID, lctx.AccountID, lctx.AgentName, body)
		if !ok {
			return
		}

		outputs, ok := resolveSubmittedOutputs(c, log, resolver, evaluationRef, body.EvaluatorOutputs)
		if !ok {
			return
		}

		item := evalitemstore.Item{
			EvalDatasetID:         ds.ID,
			TraceID:               body.TraceID,
			EvaluationRef:         evaluationRef,
			SourceEvaluationRunID: sourceRunID,
			VerifiedByUserID:      lctx.UserID,
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
			// Detached from the request so the rollback still runs when the client
			// disconnected, with a timeout so a hung DB doesn't leak forever.
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, _, deleteErr := itemStore.Remove(rollbackCtx, ds.ID, body.TraceID); deleteErr != nil {
				log.Warn("dataset items: roll back dataset item failed", "error", deleteErr, "trace_id", body.TraceID)
			}
			log.Error("dataset items: upsert Langfuse dataset item failed", "error", err, "trace_id", body.TraceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
			return
		}

		c.JSON(http.StatusCreated, DatasetItemResponse{
			EvalDatasetID: ds.ID,
			TraceID:       body.TraceID,
			EvaluationRef: evaluationRef,
		})
	}
}

type DatasetItemOutputsRequest struct {
	Values []datasetItemEvaluatorOutput `json:"values"`
}

type DatasetItemOutputsResponse struct {
	EvalDatasetID    string                 `json:"eval_dataset_id"`
	TraceID          string                 `json:"trace_id"`
	EvaluationRef    string                 `json:"evaluation_ref"`
	VerifiedByUserID string                 `json:"verified_by_user_id"`
	EvaluatorOutputs []evalitemstore.Output `json:"evaluator_outputs"`
}

// PutDatasetItemEvaluatorOutputs replaces a dataset item's final values with the
// submitted ones, dropping any evaluator the caller left out. Evaluation runs and
// their results are left in place, so the automated values stay available for
// comparison.
// PUT /api/v1/deployments/:id/dataset/items/:trace_id/evaluator-outputs
func PutDatasetItemEvaluatorOutputs(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	itemStore *evalitemstore.Store,
	resolver evalSetResolver,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		traceID, ok := requireTraceIDParam(c)
		if !ok {
			return
		}

		var body DatasetItemOutputsRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		item, err := itemStore.Get(c.Request.Context(), ds.ID, traceID)
		if err != nil {
			log.Error("dataset items: load dataset item failed", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the dataset item"})
			return
		}
		if item == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset item not found"})
			return
		}

		activeRef, err := resolver.ActiveRef(c.Request.Context(), dctx.Deployment.AccountID, dctx.Deployment.AgentName)
		if err != nil {
			log.Error("dataset items: resolve active ref failed", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return
		}
		// A retired set has no resolvable output contract to validate against, so
		// its outputs stay as admitted until the item is removed and re-added.
		if item.EvaluationRef != activeRef {
			c.JSON(http.StatusConflict, gin.H{"error": "dataset item does not use the active evaluation set"})
			return
		}

		outputs, ok := resolveSubmittedOutputs(c, log, resolver, item.EvaluationRef, body.Values)
		if !ok {
			return
		}

		if err := itemStore.ReplaceOutputs(c.Request.Context(), ds.ID, traceID, dctx.UserID, outputs); err != nil {
			if errors.Is(err, evalitemstore.ErrItemNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "dataset item not found"})
				return
			}
			log.Error("dataset items: replace evaluator outputs failed", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update the evaluator outputs"})
			return
		}

		c.JSON(http.StatusOK, DatasetItemOutputsResponse{
			EvalDatasetID:    ds.ID,
			TraceID:          traceID,
			EvaluationRef:    item.EvaluationRef,
			VerifiedByUserID: dctx.UserID,
			EvaluatorOutputs: outputs,
		})
	}
}

// DeleteDatasetItem removes a trace from the dataset along with its final
// evaluator outputs, returning the trace to the review queue. Evaluation runs
// and their results are left in place.
// DELETE /api/v1/deployments/:id/dataset/items/:trace_id
func DeleteDatasetItem(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	itemStore *evalitemstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traceID, ok := requireTraceIDParam(c)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		// Delete before the Langfuse write, mirroring the add, so a concurrent
		// retry loses here rather than racing the upstream delete.
		item, outputs, err := itemStore.Remove(c.Request.Context(), ds.ID, traceID)
		if err != nil && !errors.Is(err, evalitemstore.ErrItemNotFound) {
			log.Error("dataset items: remove dataset item failed", "error", err, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove the dataset item"})
			return
		}

		// The Langfuse ID derives from the trace rather than the local row, so the
		// delete still reaches an item the legacy judgment path or an interrupted
		// add left upstream without one.
		datasetItemID := evaldataset.ItemID(ds.LangfuseDatasetName, traceID)
		deleted, err := evaldataset.DeleteItem(c.Request.Context(), lctx.Client, datasetItemID)
		if err != nil {
			if item != nil {
				restoreCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if restoreErr := itemStore.Add(restoreCtx, *item, outputs); restoreErr != nil {
					log.Warn("dataset items: restore dataset item failed", "error", restoreErr, "trace_id", traceID)
				}
			}
			log.Error("dataset items: delete Langfuse dataset item failed", "error", err,
				"trace_id", traceID, "dataset_item_id", datasetItemID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete dataset item"})
			return
		}

		if item == nil && !deleted {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace is not in the dataset"})
			return
		}

		response := DatasetItemResponse{EvalDatasetID: ds.ID, TraceID: traceID}
		if item != nil {
			response.EvaluationRef = item.EvaluationRef
		}
		c.JSON(http.StatusOK, response)
	}
}

func resolveItemProvenance(
	c *gin.Context,
	log *logger.Logger,
	runStore *evalrunstore.Store,
	resolver evalSetResolver,
	evalDatasetID, accountID, agentName string,
	body DatasetItemRequest,
) (sourceRunID *string, evaluationRef string, ok bool) {
	runID := strings.TrimSpace(body.EvaluationRunID)
	if runID == "" {
		ref, err := resolver.ActiveRef(c.Request.Context(), accountID, agentName)
		if err != nil {
			log.Error("dataset items: resolve active ref failed", "error", err,
				"account_id", accountID, "agent_name", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return nil, "", false
		}
		return nil, ref, true
	}
	run, err := runStore.GetRun(c.Request.Context(), runID)
	if err != nil {
		log.Error("dataset items: load evaluation run failed", "error", err, "evaluation_run_id", runID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the evaluation run"})
		return nil, "", false
	}
	if run == nil || run.EvalDatasetID != evalDatasetID || run.TraceID != body.TraceID {
		c.JSON(http.StatusConflict, gin.H{"error": "evaluation run does not match this trace"})
		return nil, "", false
	}
	return &run.ID, run.EvaluationRef, true
}

func resolveSubmittedOutputs(
	c *gin.Context,
	log *logger.Logger,
	resolver evalSetResolver,
	evaluationRef string,
	submitted []datasetItemEvaluatorOutput,
) ([]evalitemstore.Output, bool) {
	set, err := resolver.Set(c.Request.Context(), evaluationRef)
	if err != nil {
		if errors.Is(err, evalresolve.ErrUnresolvable) || errors.Is(err, evalpreset.ErrUnknownRef) {
			c.JSON(http.StatusConflict, gin.H{"error": "evaluation run does not match this trace"})
			return nil, false
		}
		log.Error("dataset items: resolve evaluation set failed", "error", err, "evaluation_ref", evaluationRef)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
		return nil, false
	}
	outputs, invalid := resolveItemOutputs(set, submitted)
	if invalid != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalid})
		return nil, false
	}
	return outputs, true
}

// resolveItemOutputs pairs each evaluator in the set with its submitted value,
// returning the outputs in definition order or the reason the request failed.
// An evaluator the caller left out is absent from the result: a reviewer can
// record as few values as they are sure of, including none.
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
		raw := byKey[definition.Key]
		delete(byKey, definition.Key)
		if raw == nil {
			continue
		}
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
