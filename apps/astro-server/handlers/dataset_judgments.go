package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

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

	return evaldataset.UpsertItem(ctx, lctx.Client, evaldataset.ItemInput{
		DatasetName:    ds.LangfuseDatasetName,
		TraceID:        traceID,
		Input:          trace.Input,
		ExpectedOutput: trace.Output,
		Metadata: map[string]any{
			"judged_by_user_id": lctx.UserID,
			"judged_at":         time.Now().UTC().Format(time.RFC3339),
			"judgment_criteria": reasonsToCriteria(criteria),
		},
	})
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

		ds, ok := loadDatasetEnsured(c, log, datasetStore, lctx)
		if !ok {
			return
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
					if deleteErr := evaldataset.DeleteItem(c.Request.Context(), lctx.Client, datasetItemID); deleteErr != nil {
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
		datasetItemID := evaldataset.ItemID(ds.LangfuseDatasetName, traceID)

		if nextEffect.writeDatasetItem {
			if _, err := upsertJudgmentDatasetItem(c.Request.Context(), lctx, ds, trace, traceID, nextEffect, nil); err != nil {
				restoreJudgment("dataset item upsert failed")
				log.Error("Failed to upsert changed dataset item", "error", err, "trace_id", traceID)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to write dataset item"})
				return
			}
		} else if previousEffect.writeDatasetItem {
			if err := evaldataset.DeleteItem(c.Request.Context(), lctx.Client, datasetItemID); err != nil {
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
					if deleteErr := evaldataset.DeleteItem(c.Request.Context(), lctx.Client, datasetItemID); deleteErr != nil {
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
			datasetItemID := evaldataset.ItemID(ds.LangfuseDatasetName, traceID)
			if err := evaldataset.DeleteItem(c.Request.Context(), lctx.Client, datasetItemID); err != nil {
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
