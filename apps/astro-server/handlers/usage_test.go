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

func setupUsageRouter(omClient *openmeter.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	log := logger.New("error", "json")
	router.GET("/usage", GetAccountUsage(log, omClient))
	return router
}

func TestGetAccountUsage_NilClient(t *testing.T) {
	router := setupUsageRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountUsage_OpenMeterError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	router := setupUsageRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (fail open), got %d", rec.Code)
	}
	var resp UsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Meters) != 0 {
		t.Errorf("expected empty meters on error, got %v", resp.Meters)
	}
}

func TestGetAccountUsage_Success(t *testing.T) {
	usage := 5.0
	quota := 10.0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"entitlements": {
				"compute":          {"hasAccess": true, "usage": %f, "totalAvailableGrantAmount": %f},
				"agents":           {"hasAccess": true, "usage": 3},
				"knowledge_stores": {"hasAccess": true, "usage": 1, "totalAvailableGrantAmount": 5}
			}
		}`, usage, quota)
	}))
	defer srv.Close()

	router := setupUsageRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp UsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Meters) != 3 {
		t.Errorf("expected 3 meters, got %d: %v", len(resp.Meters), resp.Meters)
	}

	compute, ok := resp.Meters["compute"]
	if !ok {
		t.Fatal("missing compute meter")
	}
	if compute.Usage != usage {
		t.Errorf("compute usage: want %f, got %f", usage, compute.Usage)
	}
	if compute.Quota == nil || *compute.Quota != quota {
		t.Errorf("compute quota: want %f, got %v", quota, compute.Quota)
	}

	agents, ok := resp.Meters["agents"]
	if !ok {
		t.Fatal("missing agents meter")
	}
	if agents.Usage != 3 {
		t.Errorf("agents usage: want 3, got %f", agents.Usage)
	}
	if agents.Quota != nil {
		t.Errorf("agents quota should be nil (no grant amount), got %v", agents.Quota)
	}

	if _, ok := resp.Meters["knowledge_stores"]; !ok {
		t.Error("missing knowledge_stores meter")
	}
}

func TestGetAccountUsage_ResponseShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"entitlements": {}}`)
	}))
	defer srv.Close()

	router := setupUsageRouter(openmeter.NewClient(srv.URL))

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp UsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AccountID != testAccount().ID {
		t.Errorf("account_id: want %q, got %q", testAccount().ID, resp.AccountID)
	}
	if resp.PeriodStart == "" || resp.PeriodEnd == "" {
		t.Error("expected period_start and period_end to be set")
	}
	if resp.Meters == nil {
		t.Error("meters should be an empty map, not nil")
	}
}
