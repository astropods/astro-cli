package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

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

// loadDatasetEnsured loads the dataset row and heals a legacy Langfuse dataset
// name before the caller writes an item, so admission never targets a dataset
// that no longer matches the canonical name.
func loadDatasetEnsured(
	c *gin.Context,
	log *logger.Logger,
	datasetStore *evaldatasetstore.Store,
	lctx *langfuseContext,
) (*evaldatasetstore.EvalDataset, bool) {
	ds, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
	if !ok {
		return nil, false
	}
	if ds.LangfuseDatasetName == evaldataset.ExpectedName(lctx.DeploymentID) {
		return ds, true
	}

	ensured, err := evaldataset.Ensure(c.Request.Context(), datasetStore, lctx.Client, evaldataset.EnsureOptions{
		DeploymentID: lctx.DeploymentID,
	})
	if err != nil {
		log.Error("Failed to ensure eval dataset", "error", err, "deployment_id", lctx.DeploymentID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare dataset"})
		return nil, false
	}
	return ensured, true
}
