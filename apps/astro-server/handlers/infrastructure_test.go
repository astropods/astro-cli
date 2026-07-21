package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupInfrastructureRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New("error", "json")
	router.Use(injectAccount(testAccount()))
	router.GET("/usage/infrastructure", GetInfrastructureUsage(log))
	router.GET("/agents/:account/:name/usage/infrastructure", GetInfrastructureUsage(log))
	return router
}

// Metered compute usage has no data source wired yet; the endpoint returns an
// empty (zero-usage) payload with the resolved account/range.

func TestGetAccountInfrastructureUsage_Empty(t *testing.T) {
	router := setupInfrastructureRouter()

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp InfrastructureUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Usage.DeploymentCompute != 0 {
		t.Errorf("expected zero usage, got %f", resp.Usage.DeploymentCompute)
	}
	if resp.AccountID != testAccount().ID {
		t.Errorf("account_id: want %q, got %q", testAccount().ID, resp.AccountID)
	}
	if resp.From == "" || resp.To == "" {
		t.Error("expected from and to to be set")
	}
}

func TestGetAgentInfrastructureUsage_Empty(t *testing.T) {
	router := setupInfrastructureRouter()

	req := httptest.NewRequest(http.MethodGet, "/agents/acme/my-agent/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp InfrastructureUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AgentName != "my-agent" {
		t.Errorf("agent_name: want my-agent, got %q", resp.AgentName)
	}
	if resp.Usage.DeploymentCompute != 0 {
		t.Errorf("expected zero usage, got %f", resp.Usage.DeploymentCompute)
	}
}

func TestGetAccountInfrastructureUsage_FromAfterTo(t *testing.T) {
	router := setupInfrastructureRouter()

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=2026-05-20T00:00:00Z&to=2026-05-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountInfrastructureUsage_InvalidFrom(t *testing.T) {
	router := setupInfrastructureRouter()

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=not-a-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
