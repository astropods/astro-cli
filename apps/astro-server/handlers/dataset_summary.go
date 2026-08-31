package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type evalDatasetSummary struct {
	DatasetName string                        `json:"dataset_name"`
	ItemCount   int                           `json:"item_count"`
	Evaluators  []evalDatasetEvaluatorSummary `json:"evaluators"`
}

type evalDatasetEvaluatorSummary struct {
	Key          string                  `json:"key"`
	Label        string                  `json:"label"`
	Distribution []evalDatasetValueCount `json:"distribution"`
}

type evalDatasetValueCount struct {
	Value json.RawMessage `json:"value"`
	Count int             `json:"count"`
}

// GetEvalDataset returns dataset summary metadata from the local DB.
// GET /api/v1/deployments/:id/dataset
func GetEvalDataset(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	itemStore *evalitemstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		itemCount, err := itemStore.Count(c.Request.Context(), ds.ID)
		if err != nil {
			log.Error("dataset summary: count dataset items failed", "error", err,
				"dataset_id", ds.ID, "deployment_id", dctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count dataset items"})
			return
		}

		valueCounts, err := itemStore.OutputValueCounts(c.Request.Context(), ds.ID)
		if err != nil {
			log.Error("dataset summary: get evaluator output counts failed", "error", err,
				"dataset_id", ds.ID, "deployment_id", dctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset evaluator counts"})
			return
		}

		// A retired evaluator still holds values, so a set that will not resolve
		// costs labels and ordering rather than the counts themselves.
		set, err := evalpreset.ResolveSet(activeEvaluationRef)
		if err != nil {
			log.Warn("dataset summary: resolve evaluation set failed", "error", err,
				"dataset_id", ds.ID, "evaluation_ref", activeEvaluationRef)
		}

		c.JSON(http.StatusOK, evalDatasetSummary{
			DatasetName: ds.LangfuseDatasetName,
			ItemCount:   itemCount,
			Evaluators:  summaryEvaluators(set, valueCounts),
		})
	}
}

func summaryEvaluators(
	set []evaluator.Evaluator,
	valueCounts []evalitemstore.ValueCount,
) []evalDatasetEvaluatorSummary {
	groups := evaluatorsBySet(set, valueCounts,
		func(count evalitemstore.ValueCount) string { return count.EvaluatorKey })

	out := make([]evalDatasetEvaluatorSummary, 0, len(groups))
	for _, group := range groups {
		distribution := make([]evalDatasetValueCount, 0, len(group.Rows))
		for _, count := range group.Rows {
			distribution = append(distribution, evalDatasetValueCount{
				Value: count.Value,
				Count: count.Count,
			})
		}
		out = append(out, evalDatasetEvaluatorSummary{
			Key:          group.Key,
			Label:        group.label(),
			Distribution: distribution,
		})
	}
	return out
}
