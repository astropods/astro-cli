package handlers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/datasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

type datasetJobQueue interface {
	InsertDatasetSyncJob(ctx context.Context, deploymentID string) error
}

// GetEvalDataset returns dataset summary metadata from the local DB.
// GET /api/v1/deployments/:id/dataset
func GetEvalDataset(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *datasetstore.Store,
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

		ds, err := datasetStore.Get(deploymentID)
		if err != nil {
			log.Error("Failed to get dataset record", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset"})
			return
		}
		if ds == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset not yet created — sync has not run for this deployment"})
			return
		}

		var lastTraceAt *string
		if ds.LastTraceAt != nil {
			s := ds.LastTraceAt.UTC().Format(time.RFC3339)
			lastTraceAt = &s
		}
		var lastSyncedAt *string
		if ds.LastSyncedAt != nil {
			s := ds.LastSyncedAt.UTC().Format(time.RFC3339)
			lastSyncedAt = &s
		}

		c.JSON(http.StatusOK, gin.H{
			"dataset_name":   ds.LangfuseDatasetName,
			"last_trace_at":  lastTraceAt,
			"last_synced_at": lastSyncedAt,
			"item_count":     ds.ItemCount,
		})
	}
}

// TriggerEvalDatasetSync enqueues an immediate dataset sync job for the deployment.
// POST /api/v1/deployments/:id/dataset/sync
func TriggerEvalDatasetSync(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	queue datasetJobQueue,
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

		if err := queue.InsertDatasetSyncJob(c.Request.Context(), deploymentID); err != nil {
			log.Error("Failed to enqueue dataset sync job", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue sync"})
			return
		}

		c.Status(http.StatusAccepted)
	}
}

// DownloadEvalDataset streams a zip archive containing a JSONL file with all dataset items.
// GET /api/v1/deployments/:id/dataset/download
func DownloadEvalDataset(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *datasetstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		ds, err := datasetStore.Get(lctx.DeploymentID)
		if err != nil {
			log.Error("Failed to get dataset record", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get dataset"})
			return
		}
		if ds == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset not yet created"})
			return
		}

		zipName := fmt.Sprintf("dep-%s.zip", lctx.DeploymentID)
		jsonlName := fmt.Sprintf("dep-%s.jsonl", lctx.DeploymentID)

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
