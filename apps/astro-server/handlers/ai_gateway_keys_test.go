package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// TestIssueAIGatewayDevKey_NilDeps verifies the 503 short-circuit when
// AI_GATEWAY_URL is unset (provisioner and store nil). Full success-path
// coverage lives at the Provisioner level (sqlmock-driven) since the
// handler is a thin pass-through to EnsureDevKey.
func TestIssueAIGatewayDevKey_NilDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New("error", "json")
	router.POST("/keys", IssueAIGatewayDevKey(log, nil, nil, testVault(t)))

	req := httptest.NewRequest(http.MethodPost, "/keys", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Keep the symbol-import referenced so a future refactor that removes the
// aigateway import surface from this file fails loudly.
var _ = aigateway.NewProvisioner
