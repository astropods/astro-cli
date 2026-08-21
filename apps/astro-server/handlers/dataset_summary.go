package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// evalDatasetSummary is the JSON shape returned by GetEvalDataset.
type evalDatasetSummary struct {
	DatasetName    string                      `json:"dataset_name"`
	ItemCount      int                         `json:"item_count"`
	CriteriaCounts []evalDatasetCriterionCount `json:"criteria_counts"`
}

type evalDatasetCriterionCount struct {
	DimensionKey string `json:"dimension_key"`
	GoodCount    int    `json:"good_count"`
	BadCount     int    `json:"bad_count"`
}

func summaryFromRow(ds *evaldatasetstore.EvalDataset, criteriaCounts []judgmentstore.CriterionCounts) evalDatasetSummary {
	return evalDatasetSummary{
		DatasetName:    ds.LangfuseDatasetName,
		ItemCount:      ds.Total(),
		CriteriaCounts: summaryCriterionCounts(criteriaCounts),
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
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		criteriaCounts, err := judgmentStore.CriterionCounts(ds.ID)
		if err != nil {
			log.Error("dataset summary: get dataset criterion counts failed", "error", err, "dataset_id", ds.ID, "deployment_id", dctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset criteria counts"})
			return
		}

		c.JSON(http.StatusOK, summaryFromRow(ds, criteriaCounts))
	}
}
