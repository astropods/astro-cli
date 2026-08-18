package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
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
