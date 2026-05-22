package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

func setupInfrastructureRouter(omClient *openmeter.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New("error", "json")
	router.Use(injectAccount(testAccount()))
	router.GET("/usage/infrastructure", GetInfrastructureUsage(log, omClient))
	router.GET("/agents/:account/:name/usage/infrastructure", GetInfrastructureUsage(log, omClient))
	return router
}

func meterQueryResponse(value float64) string {
	return fmt.Sprintf(`{"data":[{"value":%f,"groupBy":{},"subject":"acct-test","windowStart":"2026-05-01T00:00:00Z","windowEnd":"2026-05-20T23:59:59Z"}],"from":"2026-05-01T00:00:00Z","to":"2026-05-20T23:59:59Z"}`, value)
}

func TestGetAccountInfrastructureUsage_NilClient(t *testing.T) {
	router := setupInfrastructureRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGetAccountInfrastructureUsage_OpenMeterError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (fail open), got %d", rec.Code)
	}
	var resp InfrastructureUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Usage.DeploymentCompute != 0 {
		t.Errorf("expected zero on error, got %f", resp.Usage.DeploymentCompute)
	}
}

func TestGetAccountInfrastructureUsage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meters/compute/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("subject") != testAccount().ID {
			t.Errorf("unexpected subject: %s", r.URL.Query().Get("subject"))
		}
		if r.URL.Query().Get("groupBy") != "" {
			t.Errorf("account query should not have groupBy, got %s", r.URL.Query().Get("groupBy"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, meterQueryResponse(139.14))
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

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
	if resp.Usage.DeploymentCompute != 139.14 {
		t.Errorf("deployment_compute: want 139.14, got %f", resp.Usage.DeploymentCompute)
	}
	if resp.AccountID != testAccount().ID {
		t.Errorf("account_id: want %q, got %q", testAccount().ID, resp.AccountID)
	}
	if resp.From == "" || resp.To == "" {
		t.Error("expected from and to to be set")
	}
}

func TestGetAccountInfrastructureUsage_TimeRangeParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from != "2026-05-13T00:00:00Z" {
			t.Errorf("from: want 2026-05-13T00:00:00Z, got %s", from)
		}
		if to != "2026-05-20T23:59:59Z" {
			t.Errorf("to: want 2026-05-20T23:59:59Z, got %s", to)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, meterQueryResponse(0))
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=2026-05-13T00:00:00Z&to=2026-05-20T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountInfrastructureUsage_FromAfterTo(t *testing.T) {
	router := setupInfrastructureRouter(openmeter.NewClient("http://unused"))

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=2026-05-20T00:00:00Z&to=2026-05-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAgentInfrastructureUsage_NilClient(t *testing.T) {
	router := setupInfrastructureRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/agents/acme/my-agent/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGetAgentInfrastructureUsage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("groupBy") != "agent_name" {
			t.Errorf("agent query should have groupBy=agent_name, got %q", r.URL.Query().Get("groupBy"))
		}
		if r.URL.Query().Get("filterGroupBy[agent_name]") != "my-agent" {
			t.Errorf("agent query should filter by agent_name=my-agent, got %q", r.URL.Query().Get("filterGroupBy[agent_name]"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"value":46.38,"groupBy":{"agent_name":"my-agent"},"subject":"acct-test","windowStart":"2026-05-01T00:00:00Z","windowEnd":"2026-05-20T23:59:59Z"}],"from":"2026-05-01T00:00:00Z","to":"2026-05-20T23:59:59Z"}`)
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

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
	if resp.Usage.DeploymentCompute != 46.38 {
		t.Errorf("deployment_compute: want 46.38, got %f", resp.Usage.DeploymentCompute)
	}
}

func TestGetAccountInfrastructureUsage_InvalidFrom(t *testing.T) {
	router := setupInfrastructureRouter(openmeter.NewClient("http://unused"))

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=not-a-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountInfrastructureUsage_InvalidTo(t *testing.T) {
	router := setupInfrastructureRouter(openmeter.NewClient("http://unused"))

	req := httptest.NewRequest(http.MethodGet, "/usage/infrastructure?from=2026-05-01T00:00:00Z&to=not-a-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAgentInfrastructureUsage_CrossAccountBlueprint(t *testing.T) {
	// "foreign-agent" is a blueprint originally published by another account.
	// The deploying account (testAccount) calls with their own account in the URL;
	// the query must succeed and be scoped to the deploying account.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("subject") != testAccount().ID {
			t.Errorf("subject should be deploying account %q, got %q", testAccount().ID, r.URL.Query().Get("subject"))
		}
		if r.URL.Query().Get("filterGroupBy[agent_name]") != "foreign-agent" {
			t.Errorf("filterGroupBy[agent_name]: want %q, got %q", "foreign-agent", r.URL.Query().Get("filterGroupBy[agent_name]"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"value":22.5,"groupBy":{"agent_name":"foreign-agent"},"subject":"acct-test","windowStart":"2026-05-01T00:00:00Z","windowEnd":"2026-05-20T23:59:59Z"}],"from":"2026-05-01T00:00:00Z","to":"2026-05-20T23:59:59Z"}`)
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/agents/acme/foreign-agent/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp InfrastructureUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Usage.DeploymentCompute != 22.5 {
		t.Errorf("deployment_compute: want 22.5, got %f", resp.Usage.DeploymentCompute)
	}
	if resp.AccountID != testAccount().ID {
		t.Errorf("account_id: want %q, got %q", testAccount().ID, resp.AccountID)
	}
}

func TestGetAgentInfrastructureUsage_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[],"from":"2026-05-01T00:00:00Z","to":"2026-05-20T23:59:59Z"}`)
	}))
	defer srv.Close()

	router := setupInfrastructureRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/agents/acme/my-agent/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp InfrastructureUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Usage.DeploymentCompute != 0 {
		t.Errorf("expected 0 for empty result, got %f", resp.Usage.DeploymentCompute)
	}
}
