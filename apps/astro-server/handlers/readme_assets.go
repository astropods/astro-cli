package handlers

import (
	"io"
	"mime/multipart"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	"github.com/gin-gonic/gin"
)

// ReadmeAssetsResponse maps each uploaded image's repo-relative path to its
// public CDN URL.
type ReadmeAssetsResponse struct {
	Assets map[string]string `json:"assets"`
}

// UploadReadmeAssets handles POST /api/v1/agents/:account/:name/readme-assets.
// The CLI sends each AGENT.md-referenced local image as a multipart file part
// whose field name is the image's repo-relative path (the field name, not the
// filename, since multipart strips directories from filenames). Each image is
// stored and the response maps those paths to their CDN URLs, which the CLI
// forwards to the register call so the stored AGENT.md links to them.
//
// Individual image failures are logged and skipped rather than failing the
// request: an unmapped image is simply left as its original relative reference.
func UploadReadmeAssets(log *logger.Logger, store *readmeassets.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		agentName := c.Param("name")

		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
			return
		}

		assets := make(map[string]string)
		for relPath, headers := range form.File {
			if len(assets) >= readmeassets.MaxAssets {
				log.Warn("readme assets: readme asset upload exceeded limit",
					"account", acct.Name, "agent", agentName, "limit", readmeassets.MaxAssets)
				break
			}
			if len(headers) == 0 {
				continue
			}
			fh := headers[0]
			if fh.Size > readmeassets.MaxAssetSize {
				log.Warn("readme assets: skipping oversized readme asset", "path", relPath, "size", fh.Size)
				continue
			}
			data, err := readMultipartFile(fh)
			if err != nil {
				log.Warn("readme assets: read readme asset failed", "path", relPath, "error", err)
				continue
			}
			url, err := store.Upload(c.Request.Context(), acct.Name, agentName, data)
			if err != nil {
				log.Warn("readme assets: store readme asset failed", "path", relPath, "error", err)
				continue
			}
			assets[relPath] = url
		}

		c.JSON(http.StatusOK, ReadmeAssetsResponse{Assets: assets})
	}
}

func readMultipartFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, readmeassets.MaxAssetSize+1))
}
