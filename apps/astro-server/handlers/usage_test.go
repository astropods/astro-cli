package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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

func setupUsageRouter(reporter quota.Reporter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	log := logger.New("error", "json")
	router.GET("/usage", GetAccountUsage(log, reporter))
	return router
}

func TestGetAccountUsage_NilReporter(t *testing.T) {
	router := setupUsageRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountUsage_ReporterError(t *testing.T) {
	router := setupUsageRouter(&fakeReporter{err: fmt.Errorf("boom")})

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
	reporter := &fakeReporter{report: map[string]quota.ResourceUsage{
		"blueprints":       {Used: 3, Limit: quota.Unlimited},
		"knowledge_stores": {Used: 1, Limit: 5},
	}}
	router := setupUsageRouter(reporter)

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

	// blueprints: usage 3 from quota, unlimited → no quota bar.
	blueprints, ok := resp.Meters["blueprints"]
	if !ok {
		t.Fatal("missing blueprints meter")
	}
	if blueprints.Usage != 3 {
		t.Errorf("blueprints usage: want 3, got %f", blueprints.Usage)
	}
	if blueprints.Quota != nil {
		t.Errorf("blueprints quota should be nil (unlimited), got %v", blueprints.Quota)
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
	router := setupUsageRouter(&fakeReporter{report: map[string]quota.ResourceUsage{}})

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
