package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// Tests in this file cover two behaviors the cache layer adds to the three
// Insights handlers:
//
//   1. Cache hit: when a key is present in Redis, the handler must return
//      those bytes verbatim and never touch Langfuse. Done by pointing
//      cfg.Deployment.LangfuseBaseURL at a server that fails the test if
//      any request hits it.
//   2. Graceful degradation: when Langfuse returns 500 for every sub-query,
//      the handler must return 200 OK with metrics_unavailable: true (rather
//      than 502 as it did before the PR).
//
// The deployment-store query result is shared via the depCols/depMockRow
// helpers below since all three handlers go through GetVisibleDeploymentsByAccount.

var depCols = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors",
}

// expectStandardAccountAndCreds wires the three sqlmock expectations every
// Insights handler runs before touching Langfuse: account lookup → membership
// check → Langfuse creds load. Returns the (accountStore, langfuseStore) pair
// already pointed at the configured mocks.
func expectStandardAccountAndCreds(t *testing.T) (
	*account.AccountStore, *langfuse.Store, sqlmock.Sqlmock, sqlmock.Sqlmock,
) {
	t.Helper()
	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, langfuseMock, _ := sqlmock.New()
	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(accountCols).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "",
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key",
			"langfuse_secret_key", "encrypted_data_key", "nonce", "created_at",
		}).AddRow("acct-1", "proj-1", "pk", "sk", nil, nil, now))

	return account.NewAccountStore(accountDB), langfuse.NewStore(langfuseDB), accountMock, langfuseMock
}

// expectOneDeployment sets up a depStore that returns a single live deployment.
func expectOneDeployment(t *testing.T) (*deploymentstore.Store, sqlmock.Sqlmock) {
	t.Helper()
	depDB, depMock, _ := sqlmock.New()
	now := time.Now()
	depMock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(depCols).
			AddRow("dep-1", "acct-1", nil, "code-reviewer", "b1", "ns-1", "Code Reviewer",
				"{}", nil, nil, nil, "Running", nil, nil, now, nil, now, nil, nil))
	return deploymentstore.NewStore(depDB), depMock
}

// failOnAnyCallServer fails the test on any HTTP request. Use as a sentinel
// Langfuse URL for cache-hit tests — proves the handler never falls through.
func failOnAnyCallServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected Langfuse request: %s %s", r.Method, r.URL.Path)
	}))
}

// alwaysFailServer returns 500 to every request — simulates Langfuse being
// fully unhealthy (the actual production failure mode triggering this PR).
func alwaysFailServer(_ *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func newCachingTestRouter(handler gin.HandlerFunc, route string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET(route, handler)
	return router
}

// ── /observability/summary ────────────────────────────────────────────────

func TestGetAccountLangfuseSummary_CacheHit_ReturnsCachedBytes(t *testing.T) {
	accountStore, _, _, _ := expectStandardAccountAndCreds(t)
	// langfuseStore lookup never runs because ComputeAccountSummary is never
	// called — the cache hit short-circuits the handler before that.
	// We still need the membership check, which lives above the cache check.
	langfuseStoreDB, _, _ := sqlmock.New()
	langfuseStore := langfuse.NewStore(langfuseStoreDB)

	cache := mapCache{}
	const cachedBody = `{"period":{"days":0},"totals":{"cost_usd":12.5,"requests":42},"metrics_unavailable":false}`
	if err := insightscache.Put(t.Context(), cache, "acct-1",
		insightscache.EndpointSummary,
		insightscache.Params{GroupBy: "user", IncludeArchived: false},
		[]byte(cachedBody)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	failSrv := failOnAnyCallServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountLangfuseSummary(logger.New("error", "json"), cfg, accountStore, nil, langfuseStore, cache),
		"/api/v1/accounts/:account/observability/summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/summary?group_by=user", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != cachedBody {
		t.Errorf("response body = %q, want exactly the cached bytes %q", got, cachedBody)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json prefix", ct)
	}
}

func TestGetAccountLangfuseSummary_LangfuseFails_DegradesTo200WithFlag(t *testing.T) {
	accountStore, langfuseStore, _, _ := expectStandardAccountAndCreds(t)
	depStore, _ := expectOneDeployment(t)

	failSrv := alwaysFailServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountLangfuseSummary(logger.New("error", "json"), cfg, accountStore, depStore, langfuseStore, nil),
		"/api/v1/accounts/:account/observability/summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/summary?group_by=user", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded), got body: %s", rec.Code, rec.Body.String())
	}
	var resp AccountObservabilitySummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.MetricsUnavailable {
		t.Errorf("MetricsUnavailable = false, want true on full Langfuse failure: %s", rec.Body.String())
	}
	// The degraded payload renders the page; payload itself is zeroed.
	if resp.Totals.Requests != 0 {
		t.Errorf("Totals.Requests = %d, want 0 on degraded response", resp.Totals.Requests)
	}
}

// TestGetAccountLangfuseSummary_PartialLangfuseFailure_NoBanner exercises
// the "any-failed vs all-failed" semantic the review bot flagged. With
// group_by=user, two errgroup goroutines can return errors — accountDailyMetrics
// (the main KPIs) and the userId-grouped query (the active-users chart).
// If only the user-grouped query fails, the page must still render with the
// main metrics intact and NO banner — banner is reserved for the all-failed
// case (typical Langfuse outage).
func TestGetAccountLangfuseSummary_PartialLangfuseFailure_NoBanner(t *testing.T) {
	accountStore, langfuseStore, _, _ := expectStandardAccountAndCreds(t)
	depStore, _ := expectOneDeployment(t)

	// Mixed server: succeed on the tags-grouped and providedModelName-grouped
	// queries (the "main" data path inside accountDailyMetrics), fail only on
	// the userId-grouped query. This matches a transient per-query failure
	// (rate-limit, single-worker timeout) — the worst case the bot called out.
	mixedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, `"field":"userId"`) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer mixedSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = mixedSrv.URL

	router := newCachingTestRouter(
		GetAccountLangfuseSummary(logger.New("error", "json"), cfg, accountStore, depStore, langfuseStore, nil),
		"/api/v1/accounts/:account/observability/summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/summary?group_by=user", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp AccountObservabilitySummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Banner stays off — only one of two tallied queries failed.
	if resp.MetricsUnavailable {
		t.Errorf("MetricsUnavailable = true on partial failure; banner should not fire when main metrics succeeded")
	}
	// The userId-grouped chart is empty since its query failed; that's expected
	// and is the intended degradation mode for partial failure.
	if len(resp.CostOverTimeByUser) != 0 {
		t.Errorf("CostOverTimeByUser len = %d, want 0 when its underlying query failed", len(resp.CostOverTimeByUser))
	}
}

// ── /observability/deployments-summary ────────────────────────────────────

func TestGetAccountDeploymentsSummary_CacheHit_ReturnsCachedBytes(t *testing.T) {
	accountStore, _, _, _ := expectStandardAccountAndCreds(t)
	langfuseStoreDB, _, _ := sqlmock.New()
	langfuseStore := langfuse.NewStore(langfuseStoreDB)

	cache := mapCache{}
	const cachedBody = `{"deployments":[{"deployment_id":"dep-cached","agent_name":"x","cost_usd":3.14}],"period":{"days":0}}`
	if err := insightscache.Put(t.Context(), cache, "acct-1",
		insightscache.EndpointDeploymentsSummary,
		insightscache.Params{IncludeArchived: false},
		[]byte(cachedBody)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	failSrv := failOnAnyCallServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountDeploymentsSummary(logger.New("error", "json"), cfg, accountStore, nil, langfuseStore, cache),
		"/api/v1/accounts/:account/observability/deployments-summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/deployments-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got, _ := io.ReadAll(rec.Body); string(got) != cachedBody {
		t.Errorf("response body = %q, want exactly the cached bytes %q", got, cachedBody)
	}
}

func TestGetAccountDeploymentsSummary_AllLangfuseFails_DegradesWithFlag(t *testing.T) {
	accountStore, langfuseStore, _, _ := expectStandardAccountAndCreds(t)
	depStore, _ := expectOneDeployment(t)

	failSrv := alwaysFailServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountDeploymentsSummary(logger.New("error", "json"), cfg, accountStore, depStore, langfuseStore, nil),
		"/api/v1/accounts/:account/observability/deployments-summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/deployments-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded): %s", rec.Code, rec.Body.String())
	}
	var resp AccountDeploymentsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.MetricsUnavailable {
		t.Errorf("MetricsUnavailable = false, want true when every Langfuse call failed: %s", rec.Body.String())
	}
	if len(resp.Deployments) != 0 {
		t.Errorf("Deployments len = %d, want 0 on degraded response", len(resp.Deployments))
	}
}

// ── /observability/users-summary ──────────────────────────────────────────

func TestGetAccountUsersSummary_CacheHit_ReturnsCachedBytes(t *testing.T) {
	accountStore, _, _, _ := expectStandardAccountAndCreds(t)
	langfuseStoreDB, _, _ := sqlmock.New()
	langfuseStore := langfuse.NewStore(langfuseStoreDB)

	cache := mapCache{}
	const cachedBody = `{"users":[{"user_id":"u_cached","requests":7}],"period":{"days":0}}`
	if err := insightscache.Put(t.Context(), cache, "acct-1",
		insightscache.EndpointUsersSummary,
		insightscache.Params{},
		[]byte(cachedBody)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	failSrv := failOnAnyCallServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountUsersSummary(logger.New("error", "json"), cfg, accountStore, nil, langfuseStore, nil, cache),
		"/api/v1/accounts/:account/observability/users-summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/users-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != cachedBody {
		t.Errorf("response body = %q, want exactly the cached bytes %q", got, cachedBody)
	}
}

func TestGetAccountUsersSummary_LangfuseFails_DegradesWithFlag(t *testing.T) {
	accountStore, langfuseStore, _, _ := expectStandardAccountAndCreds(t)
	depStore, _ := expectOneDeployment(t)

	failSrv := alwaysFailServer(t)
	defer failSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = failSrv.URL

	router := newCachingTestRouter(
		GetAccountUsersSummary(logger.New("error", "json"), cfg, accountStore, depStore, langfuseStore, nil, nil),
		"/api/v1/accounts/:account/observability/users-summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/users-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded): %s", rec.Code, rec.Body.String())
	}
	var resp AccountUsersSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.MetricsUnavailable {
		t.Errorf("MetricsUnavailable = false, want true on Langfuse failure: %s", rec.Body.String())
	}
}

// Users-summary fans out to two Langfuse queries:
//   - Q_main  — per-user metrics (cost/tokens/requests), has timeDimension
//   - Q_tags  — per-user deployment tag attribution, no timeDimension
//
// Q_main is the only carrier of actual metrics. When it fails but Q_tags
// succeeds, the legacy "all-failed" tally let the response through with
// users discovered via Q_tags carrying $0 cost / 0 requests, and the
// refresh worker happily cached those zeros for the next 6h. Guard pins
// the inverted behavior: any Q_main failure must degrade to the
// metrics_unavailable banner so the worker preserves the prior cache.
func TestGetAccountUsersSummary_QMainFails_QTagsSucceeds_DegradesWithFlag(t *testing.T) {
	accountStore, langfuseStore, _, _ := expectStandardAccountAndCreds(t)
	depStore, _ := expectOneDeployment(t)

	mixedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		// Q_main carries timeDimension; Q_tags doesn't. Fail only Q_main.
		if strings.Contains(q, `"timeDimension"`) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"userId":"U07ABC","tags":"deployment:dep-1","count_count":3}]}`))
	}))
	defer mixedSrv.Close()
	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = mixedSrv.URL

	router := newCachingTestRouter(
		GetAccountUsersSummary(logger.New("error", "json"), cfg, accountStore, depStore, langfuseStore, nil, nil),
		"/api/v1/accounts/:account/observability/users-summary",
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/users-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded): %s", rec.Code, rec.Body.String())
	}
	var resp AccountUsersSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.MetricsUnavailable {
		t.Errorf("MetricsUnavailable = false, want true: Q_main failure must degrade so the worker doesn't cache zeros. body=%s",
			rec.Body.String())
	}
	if len(resp.Users) != 0 {
		t.Errorf("Users len = %d, want 0 on degraded response (any user list here would be all-zero metrics)",
			len(resp.Users))
	}
}
