package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// mapCache (the package-private one defined in github_account_test.go) is
// reused here for seeding obs summary entries before exercising the handler.

func seedCache(t *testing.T, c k8scache.Cache, id string, entry *obssummary.Entry) {
	t.Helper()
	if err := obssummary.Put(context.Background(), c, id, entry); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
}

// expectActiveDeployments sets up the sqlmock for one
// GetActiveDeploymentsByAccount call returning the given deployment ids.
func expectActiveDeployments(mock sqlmock.Sqlmock, accountID string, deploymentIDs ...string) {
	cols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id",
		"namespace", "display_name", "deployment_spec_json", "status",
		"deployed_at", "undeployed_at",
	}
	rows := sqlmock.NewRows(cols)
	now := time.Now()
	for _, id := range deploymentIDs {
		rows.AddRow(id, accountID, nil, "agent-"+id, "build-1", "ns", id+"-display", []byte("{}"), "active", now, nil)
	}
	mock.ExpectQuery("SELECT id, account_id, source_account_id, agent_name").
		WithArgs(accountID).
		WillReturnRows(rows)
}

// runSummariesRequest wires the handler with an account already set in
// context and returns the recorder for assertions.
func runSummariesRequest(t *testing.T, deploymentStore *deploymentstore.Store, cache k8scache.Cache, acct *account.Account) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), acct)
		c.Next()
	})
	log := logger.New("error", "json")
	router.GET("/api/v1/accounts/:account/observability/deployment-summaries",
		GetLangfuseSummaries(log, deploymentStore, cache))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/deployment-summaries", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetLangfuseSummaries_ReadsFromCache(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	expectActiveDeployments(mock, "acct-1", "dep-A", "dep-B")

	cache := mapCache{}
	seedCache(t, cache, "dep-A", &obssummary.Entry{
		TotalTraces:   100,
		LastTraceAt:   "2026-05-27T10:00:00Z",
		CostUSD:       1.25,
		RequestSeries: []int{1, 2, 3},
		TokenSeries:   []int{100, 200, 300},
		CostSeries:    []float64{0.25, 0.4, 0.6},
	})
	seedCache(t, cache, "dep-B", &obssummary.Entry{
		TotalTraces:   5,
		LastTraceAt:   "2026-05-26T09:00:00Z",
		CostUSD:       0.75,
		RequestSeries: []int{0, 0, 5},
		TokenSeries:   []int{0, 0, 500},
		CostSeries:    []float64{0, 0, 0.75},
	})

	rec := runSummariesRequest(t, deploymentstore.NewStore(db), cache, &account.Account{ID: "acct-1", Name: "myorg"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Summaries map[string]struct {
			TotalTraces   int       `json:"total_traces"`
			LastTraceAt   string    `json:"last_trace_at"`
			CostUSD       float64   `json:"cost_usd"`
			RequestSeries []int     `json:"request_series"`
			TokenSeries   []int     `json:"token_series"`
			CostSeries    []float64 `json:"cost_series"`
		} `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Summaries) != 2 {
		t.Errorf("got %d summaries, want 2", len(resp.Summaries))
	}
	if resp.Summaries["dep-A"].TotalTraces != 100 {
		t.Errorf("dep-A TotalTraces = %d, want 100", resp.Summaries["dep-A"].TotalTraces)
	}
	if got := resp.Summaries["dep-B"].RequestSeries; len(got) != 3 || got[2] != 5 {
		t.Errorf("dep-B RequestSeries = %v, want [0 0 5]", got)
	}
	if got := resp.Summaries["dep-A"].CostUSD; got != 1.25 {
		t.Errorf("dep-A CostUSD = %v, want 1.25", got)
	}
	if got := resp.Summaries["dep-B"].CostSeries; len(got) != 3 || got[2] != 0.75 {
		t.Errorf("dep-B CostSeries = %v, want [0 0 0.75]", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetLangfuseSummaries_OmitsMissingCacheEntries(t *testing.T) {
	// The page lists 3 deployments but only 2 have cache entries. The third
	// is silently omitted so the frontend hides its sparkline.
	db, mock, _ := sqlmock.New()
	defer db.Close()
	expectActiveDeployments(mock, "acct-1", "dep-A", "dep-B", "dep-C")

	cache := mapCache{}
	seedCache(t, cache, "dep-A", &obssummary.Entry{TotalTraces: 1, RequestSeries: []int{1}, TokenSeries: []int{1}})
	seedCache(t, cache, "dep-C", &obssummary.Entry{TotalTraces: 3, RequestSeries: []int{3}, TokenSeries: []int{3}})

	rec := runSummariesRequest(t, deploymentstore.NewStore(db), cache, &account.Account{ID: "acct-1", Name: "myorg"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Summaries map[string]any `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp.Summaries["dep-A"]; !ok {
		t.Error("dep-A missing from response")
	}
	if _, ok := resp.Summaries["dep-B"]; ok {
		t.Error("dep-B should be omitted (no cache entry)")
	}
	if _, ok := resp.Summaries["dep-C"]; !ok {
		t.Error("dep-C missing from response")
	}
}

func TestGetLangfuseSummaries_NoDeployments_ReturnsEmpty(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	expectActiveDeployments(mock, "acct-1") // no rows

	rec := runSummariesRequest(t, deploymentstore.NewStore(db), mapCache{}, &account.Account{ID: "acct-1", Name: "myorg"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Summaries map[string]any `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Summaries) != 0 {
		t.Errorf("expected empty summaries map, got %v", resp.Summaries)
	}
}
