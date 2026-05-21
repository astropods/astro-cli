package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

func setupInfrastructureRouter(omClient *openmeter.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New("error", "json")
	router.Use(injectAccount(testAccount()))
	router.GET("/usage/infrastructure", GetInfrastructureUsage(log, omClient, nil))
	router.GET("/agents/:account/:name/usage/infrastructure", GetInfrastructureUsage(log, omClient, nil))
	return router
}

func setupInfrastructureRouterWithIndex(omClient *openmeter.Client, index *agentindex.Index) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := logger.New("error", "json")
	router.Use(injectAccount(testAccount()))
	router.GET("/agents/:account/:name/usage/infrastructure", GetInfrastructureUsage(log, omClient, index))
	return router
}

// indexWithAgent returns an agentindex backed by a sqlmock DB. When found is true, the
// mock expects index.Get to return the named agent; when false, it returns no rows (not found).
func indexWithAgent(t *testing.T, agentName string, found bool) (*agentindex.Index, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cols := []string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols)
	if found {
		now := time.Now()
		rows.AddRow(testAccount().ID, agentName, "registry.example.com", "private", nil, false, nil, now, now)
		mock.ExpectQuery("SELECT account_id, name").
			WithArgs(testAccount().ID, agentName).
			WillReturnRows(rows)
		mock.ExpectQuery("SELECT build_id").
			WithArgs(testAccount().ID, agentName).
			WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}))
	} else {
		mock.ExpectQuery("SELECT account_id, name").
			WithArgs(testAccount().ID, agentName).
			WillReturnRows(rows)
	}

	return agentindex.NewIndexWithDB(db), mock
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

	index, _ := indexWithAgent(t, "my-agent", true)
	router := setupInfrastructureRouterWithIndex(openmeter.NewClient(srv.URL), index)

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

func TestGetAgentInfrastructureUsage_NotFound(t *testing.T) {
	index, _ := indexWithAgent(t, "my-agent", false)
	router := setupInfrastructureRouterWithIndex(openmeter.NewClient("http://unused"), index)

	req := httptest.NewRequest(http.MethodGet, "/agents/acme/my-agent/usage/infrastructure", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
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
