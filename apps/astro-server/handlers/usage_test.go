package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/gin-gonic/gin"
)

// fakeReporter is a test double for quota.Reporter.
type fakeReporter struct {
	report map[string]quota.ResourceUsage
	err    error
}

func (f *fakeReporter) Report(_ context.Context, _ string, _ ...string) (map[string]quota.ResourceUsage, error) {
	return f.report, f.err
}

func setupUsageRouter(omClient *openmeter.Client, reporter quota.Reporter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	log := logger.New("error", "json")
	router.GET("/usage", GetAccountUsage(log, omClient, reporter))
	return router
}

func TestGetAccountUsage_NilClient(t *testing.T) {
	router := setupUsageRouter(nil, nil)

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

	router := setupUsageRouter(openmeter.NewClient(srv.URL), nil)

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
	quotaLimit := 10.0
	// OpenMeter serves consumption meters only; count features come from quota.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"entitlements": {
				"compute":          {"hasAccess": true, "usage": %f, "totalAvailableGrantAmount": %f},
				"agents":           {"hasAccess": true, "usage": 99}
			}
		}`, usage, quotaLimit)
	}))
	defer srv.Close()

	reporter := &fakeReporter{report: map[string]quota.ResourceUsage{
		"agents":           {Used: 3, Limit: quota.Unlimited},
		"knowledge_stores": {Used: 1, Limit: 5},
	}}
	router := setupUsageRouter(openmeter.NewClient(srv.URL), reporter)

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

	// compute (from OpenMeter) + agents + knowledge_stores (from quota).
	compute, ok := resp.Meters["compute"]
	if !ok {
		t.Fatal("missing compute meter")
	}
	if compute.Usage != usage {
		t.Errorf("compute usage: want %f, got %f", usage, compute.Usage)
	}
	if compute.Quota == nil || *compute.Quota != quotaLimit {
		t.Errorf("compute quota: want %f, got %v", quotaLimit, compute.Quota)
	}

	// agents comes from quota (3), NOT the OpenMeter value (99). Unlimited → no quota bar.
	agents, ok := resp.Meters["agents"]
	if !ok {
		t.Fatal("missing agents meter")
	}
	if agents.Usage != 3 {
		t.Errorf("agents usage: want 3 (from quota), got %f", agents.Usage)
	}
	if agents.Quota != nil {
		t.Errorf("agents quota should be nil (unlimited), got %v", agents.Quota)
	}

	ks, ok := resp.Meters["knowledge_stores"]
	if !ok {
		t.Fatal("missing knowledge_stores meter")
	}
	if ks.Usage != 1 || ks.Quota == nil || *ks.Quota != 5 {
		t.Errorf("knowledge_stores: want usage 1 limit 5, got %f / %v", ks.Usage, ks.Quota)
	}
}

func TestGetAccountUsage_ResponseShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"entitlements": {}}`)
	}))
	defer srv.Close()

	router := setupUsageRouter(openmeter.NewClient(srv.URL), nil)

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
