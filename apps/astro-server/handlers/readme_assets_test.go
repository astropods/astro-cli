package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	"github.com/gin-gonic/gin"
)

type fakeReadmeBackend struct{ writes map[string][]byte }

func (b *fakeReadmeBackend) Write(_ context.Context, key string, data []byte, _ string) error {
	b.writes[key] = data
	return nil
}

func (b *fakeReadmeBackend) Exists(_ context.Context, key string) (bool, error) {
	_, ok := b.writes[key]
	return ok, nil
}

// TestUploadReadmeAssets_PreservesPathAsFieldName verifies the load-bearing
// contract: the repo-relative path travels as the multipart field name (which
// gin preserves verbatim, unlike the filename which it reduces to a basename),
// so a nested path round-trips into the response map intact.
func TestUploadReadmeAssets_PreservesPathAsFieldName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := readmeassets.NewStore(&fakeReadmeBackend{writes: map[string][]byte{}}, "https://assets.example")
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/agents/:account/:name/readme-assets", func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acc-1", Name: "acc"})
		UploadReadmeAssets(log, store)(c)
	})

	pngBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("docs/sub/diagram.png", "diagram.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/agents/acc/agent/readme-assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp ReadmeAssetsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	url, ok := resp.Assets["docs/sub/diagram.png"]
	if !ok {
		t.Fatalf("expected nested path key preserved, got %v", resp.Assets)
	}
	if wantPrefix := "https://assets.example/readme-assets/acc/agent/"; url[:len(wantPrefix)] != wantPrefix {
		t.Errorf("url %q missing expected prefix %q", url, wantPrefix)
	}
}
