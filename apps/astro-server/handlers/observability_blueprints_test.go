package handlers

import (
	"encoding/json"
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
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// ── buildBlueprintsSummary ────────────────────────────────────────────────────

func TestBuildBlueprintsSummary_Empty(t *testing.T) {
	entries := buildBlueprintsSummary(nil)
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

func TestBuildBlueprintsSummary_SingleDeployment(t *testing.T) {
	metrics := []deploymentMetrics{
		{
			AgentName: "code-reviewer",
			DailyMetrics: []langfuse.DailyMetric{
				{
					Date: "2026-05-01", CountTraces: 100, TotalCost: 5.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-sonnet", InputUsage: 1000, OutputUsage: 500, TotalCost: 5.0},
					},
				},
			},
			P95LatencyMs: 800,
		},
	}

	entries := buildBlueprintsSummary(metrics)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.AgentName != "code-reviewer" {
		t.Errorf("agent_name = %q, want %q", e.AgentName, "code-reviewer")
	}
	if e.Requests != 100 {
		t.Errorf("requests = %d, want 100", e.Requests)
	}
	if e.InputTokens != 1000 {
		t.Errorf("input_tokens = %d, want 1000", e.InputTokens)
	}
	if e.OutputTokens != 500 {
		t.Errorf("output_tokens = %d, want 500", e.OutputTokens)
	}
	if e.TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want %q", e.TopModel, "claude-sonnet")
	}
	if e.P95LatencyMs != 800 {
		t.Errorf("p95_latency_ms = %d, want 800", e.P95LatencyMs)
	}
}

func TestBuildBlueprintsSummary_MultipleDeploymentsSameBlueprint(t *testing.T) {
	// Two deployments of the same agent_name (e.g. two regions) should be merged.
	metrics := []deploymentMetrics{
		{
			AgentName: "summarizer",
			DailyMetrics: []langfuse.DailyMetric{
				{Date: "2026-05-01", CountTraces: 50, TotalCost: 2.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", InputUsage: 400, OutputUsage: 100, TotalCost: 2.0},
					}},
			},
			P95LatencyMs: 300,
		},
		{
			AgentName: "summarizer",
			DailyMetrics: []langfuse.DailyMetric{
				{Date: "2026-05-01", CountTraces: 30, TotalCost: 1.5,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", InputUsage: 200, OutputUsage: 80, TotalCost: 1.5},
					}},
			},
			P95LatencyMs: 500, // higher — should win
		},
	}

	entries := buildBlueprintsSummary(metrics)
	if len(entries) != 1 {
		t.Fatalf("expected 1 merged entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Requests != 80 {
		t.Errorf("requests = %d, want 80", e.Requests)
	}
	if e.InputTokens != 600 {
		t.Errorf("input_tokens = %d, want 600", e.InputTokens)
	}
	if e.P95LatencyMs != 500 {
		t.Errorf("p95_latency_ms = %d, want 500 (max across deployments)", e.P95LatencyMs)
	}
}

func TestBuildBlueprintsSummary_SortByCostDesc(t *testing.T) {
	metrics := []deploymentMetrics{
		{AgentName: "cheap-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 10, TotalCost: 1.0},
		}},
		{AgentName: "expensive-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 100, TotalCost: 9.0},
		}},
		{AgentName: "mid-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 50, TotalCost: 4.5},
		}},
	}

	entries := buildBlueprintsSummary(metrics)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].AgentName != "expensive-agent" {
		t.Errorf("entries[0] = %q, want expensive-agent", entries[0].AgentName)
	}
	if entries[1].AgentName != "mid-agent" {
		t.Errorf("entries[1] = %q, want mid-agent", entries[1].AgentName)
	}
	if entries[2].AgentName != "cheap-agent" {
		t.Errorf("entries[2] = %q, want cheap-agent", entries[2].AgentName)
	}
}

func TestBuildBlueprintsSummary_ZeroRequestsGuard(t *testing.T) {
	// Deployment with no traces — cost_per_request and tok_per_request must be 0, not +Inf.
	metrics := []deploymentMetrics{
		{AgentName: "idle-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 0, TotalCost: 0},
		}},
	}

	entries := buildBlueprintsSummary(metrics)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.CostPerRequest != 0 {
		t.Errorf("cost_per_request = %v, want 0 (no division by zero)", e.CostPerRequest)
	}
	if e.TokPerRequest != 0 {
		t.Errorf("tok_per_request = %v, want 0 (no division by zero)", e.TokPerRequest)
	}
	// Verify JSON-marshals without issue (encoding/json would write null for +Inf).
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(b) == "" {
		t.Error("expected non-empty JSON")
	}
}

func TestBuildBlueprintsSummary_TopModel(t *testing.T) {
	metrics := []deploymentMetrics{
		{
			AgentName: "multi-model-agent",
			DailyMetrics: []langfuse.DailyMetric{
				{CountTraces: 10, TotalCost: 6.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", TotalCost: 1.0},
						{Model: "claude-sonnet", TotalCost: 5.0}, // highest
					}},
			},
		},
	}

	entries := buildBlueprintsSummary(metrics)
	if entries[0].TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want claude-sonnet", entries[0].TopModel)
	}
}

func TestBuildBlueprintsSummary_TopModelMergedAcrossDeployments(t *testing.T) {
	// Haiku is dominant in dep-1, Sonnet is dominant in dep-2; sonnet wins on total.
	metrics := []deploymentMetrics{
		{
			AgentName: "shared-agent",
			DailyMetrics: []langfuse.DailyMetric{
				{CountTraces: 5, TotalCost: 2.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", TotalCost: 2.0},
					}},
			},
		},
		{
			AgentName: "shared-agent",
			DailyMetrics: []langfuse.DailyMetric{
				{CountTraces: 20, TotalCost: 8.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-sonnet", TotalCost: 8.0},
					}},
			},
		},
	}

	entries := buildBlueprintsSummary(metrics)
	if entries[0].TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want claude-sonnet (highest cumulative cost)", entries[0].TopModel)
	}
}

func TestBuildBlueprintsSummary_CostPerRequestRounding(t *testing.T) {
	metrics := []deploymentMetrics{
		{AgentName: "agent-a", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 3, TotalCost: 1.0},
		}},
	}

	entries := buildBlueprintsSummary(metrics)
	// 1.0 / 3 = 0.3333... → rounded to 4dp = 0.3333
	if entries[0].CostPerRequest != 0.3333 {
		t.Errorf("cost_per_request = %v, want 0.3333", entries[0].CostPerRequest)
	}
}

// ── GetAccountBlueprintsSummary handler ───────────────────────────────────────

func TestGetAccountBlueprintsSummary_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, langfuseMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	langfuseStore := langfuse.NewStore(langfuseDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	now := time.Now()
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(accountCols).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "encrypted_data_key", "nonce", "created_at"}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/blueprints-summary",
		GetAccountBlueprintsSummary(log, cfg, accountStore, nil, langfuseStore))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/blueprints-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AccountBlueprintsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Blueprints == nil {
		t.Error("blueprints should be empty slice, not nil")
	}
	if len(resp.Blueprints) != 0 {
		t.Errorf("blueprints len = %d, want 0", len(resp.Blueprints))
	}
}

func TestGetAccountBlueprintsSummary_InvalidPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, _, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	langfuseStore := langfuse.NewStore(langfuseDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	// Validation only fires when both params are present — mirrors GetAccountLangfuseSummary behavior.
	now := time.Now()
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(accountCols).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/blueprints-summary",
		GetAccountBlueprintsSummary(log, cfg, accountStore, nil, langfuseStore))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/blueprints-summary?from=not-a-date&to=also-not-a-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid from+to, got %d", rec.Code)
	}
}

func TestGetAccountBlueprintsSummary_HappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	langfuseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/api/public/metrics/daily") {
			json.NewEncoder(w).Encode(langfuse.DailyMetricsResponse{ //nolint:errcheck
				Data: []langfuse.DailyMetric{
					{Date: "2026-05-01", CountTraces: 10, TotalCost: 2.5,
						Usage: []langfuse.DailyMetricUsage{
							{Model: "claude-sonnet", InputUsage: 500, OutputUsage: 100, TotalCost: 2.5},
						}},
				},
				Meta: struct {
					Page       int `json:"page"`
					Limit      int `json:"limit"`
					TotalItems int `json:"totalItems"`
					TotalPages int `json:"totalPages"`
				}{Page: 1, Limit: 50, TotalItems: 1, TotalPages: 1},
			})
		} else {
			json.NewEncoder(w).Encode(langfuse.MetricsResponse{ //nolint:errcheck
				Data: []map[string]any{{"p95_latency": 1200.0}},
			})
		}
	}))
	defer langfuseSrv.Close()

	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, langfuseMock, _ := sqlmock.New()
	depDB, depMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	langfuseStore := langfuse.NewStore(langfuseDB)
	depStore := deploymentstore.NewStore(depDB)

	now := time.Now()
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(accountCols).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "encrypted_data_key", "nonce", "created_at"}).
			AddRow("acct-1", "proj-1", "pk", "sk", nil, nil, now))

	depCols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors",
	}
	depMock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(depCols).
			AddRow("dep-1", "acct-1", nil, "code-reviewer", "b1", "ns-1", "Code Reviewer",
				"{}", nil, nil, "Running", nil, nil, now, nil, now, nil, nil))

	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = langfuseSrv.URL

	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/blueprints-summary",
		GetAccountBlueprintsSummary(log, cfg, accountStore, depStore, langfuseStore))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/blueprints-summary?from=2026-05-01T00:00:00Z&to=2026-05-08T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AccountBlueprintsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Blueprints) != 1 {
		t.Fatalf("expected 1 blueprint, got %d: %s", len(resp.Blueprints), rec.Body.String())
	}
	b := resp.Blueprints[0]
	if b.AgentName != "code-reviewer" {
		t.Errorf("agent_name = %q, want code-reviewer", b.AgentName)
	}
	if b.Requests != 10 {
		t.Errorf("requests = %d, want 10", b.Requests)
	}
	if b.P95LatencyMs != 1200 {
		t.Errorf("p95_latency_ms = %d, want 1200", b.P95LatencyMs)
	}
	if b.TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want claude-sonnet", b.TopModel)
	}
	if resp.Period.Start != "2026-05-01T00:00:00Z" {
		t.Errorf("period.start = %q, want 2026-05-01T00:00:00Z", resp.Period.Start)
	}
}
