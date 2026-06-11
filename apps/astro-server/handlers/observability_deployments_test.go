package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
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

// ── buildDeploymentSummary ────────────────────────────────────────────────────

func TestBuildDeploymentSummary_Empty(t *testing.T) {
	entries := buildDeploymentSummary(nil, nil, nil, nil)
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

func TestBuildDeploymentSummary_SingleDeployment(t *testing.T) {
	metrics := []deploymentMetrics{
		{
			DeploymentID: "dep-1",
			AgentName:    "code-reviewer",
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
	deployments := []*deploymentstore.Deployment{
		{ID: "dep-1", AgentName: "code-reviewer", DisplayName: "Code Reviewer", Namespace: "us-east-1"},
	}

	entries := buildDeploymentSummary(metrics, nil, deployments, nil)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.DeploymentID != "dep-1" {
		t.Errorf("deployment_id = %q, want dep-1", e.DeploymentID)
	}
	if e.AgentName != "code-reviewer" {
		t.Errorf("agent_name = %q, want code-reviewer", e.AgentName)
	}
	if e.DisplayName != "Code Reviewer" {
		t.Errorf("display_name = %q, want Code Reviewer", e.DisplayName)
	}
	if e.Namespace != "us-east-1" {
		t.Errorf("namespace = %q, want us-east-1", e.Namespace)
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
		t.Errorf("top_model = %q, want claude-sonnet", e.TopModel)
	}
	if e.P95LatencyMs != 800 {
		t.Errorf("p95_latency_ms = %d, want 800", e.P95LatencyMs)
	}
}

func TestBuildDeploymentSummary_MultipleDeploymentsSameAgentName(t *testing.T) {
	// Two deployments of the same agent_name (e.g. two regions) must surface
	// as TWO separate entries — no rollup. Per-deployment P95 is preserved.
	metrics := []deploymentMetrics{
		{
			DeploymentID: "dep-east",
			AgentName:    "summarizer",
			DailyMetrics: []langfuse.DailyMetric{
				{Date: "2026-05-01", CountTraces: 50, TotalCost: 2.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", InputUsage: 400, OutputUsage: 100, TotalCost: 2.0},
					}},
			},
			P95LatencyMs: 300,
		},
		{
			DeploymentID: "dep-west",
			AgentName:    "summarizer",
			DailyMetrics: []langfuse.DailyMetric{
				{Date: "2026-05-01", CountTraces: 30, TotalCost: 1.5,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", InputUsage: 200, OutputUsage: 80, TotalCost: 1.5},
					}},
			},
			P95LatencyMs: 500,
		},
	}
	deployments := []*deploymentstore.Deployment{
		{ID: "dep-east", AgentName: "summarizer", Namespace: "us-east-1"},
		{ID: "dep-west", AgentName: "summarizer", Namespace: "us-west-2"},
	}

	entries := buildDeploymentSummary(metrics, nil, deployments, nil)
	if len(entries) != 2 {
		t.Fatalf("expected 2 separate entries (no rollup), got %d", len(entries))
	}
	// Cost-desc sort puts dep-east first ($2.00 vs $1.50).
	if entries[0].DeploymentID != "dep-east" {
		t.Errorf("entries[0].deployment_id = %q, want dep-east", entries[0].DeploymentID)
	}
	if entries[0].P95LatencyMs != 300 {
		t.Errorf("entries[0].p95 = %d, want 300 (own value, no max)", entries[0].P95LatencyMs)
	}
	if entries[1].P95LatencyMs != 500 {
		t.Errorf("entries[1].p95 = %d, want 500 (own value)", entries[1].P95LatencyMs)
	}
}

func TestBuildDeploymentSummary_SortByCostDesc(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-cheap", AgentName: "cheap-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 10, TotalCost: 1.0},
		}},
		{DeploymentID: "dep-expensive", AgentName: "expensive-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 100, TotalCost: 9.0},
		}},
		{DeploymentID: "dep-mid", AgentName: "mid-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 50, TotalCost: 4.5},
		}},
	}

	entries := buildDeploymentSummary(metrics, nil, nil, nil)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].DeploymentID != "dep-expensive" {
		t.Errorf("entries[0] = %q, want dep-expensive", entries[0].DeploymentID)
	}
	if entries[1].DeploymentID != "dep-mid" {
		t.Errorf("entries[1] = %q, want dep-mid", entries[1].DeploymentID)
	}
	if entries[2].DeploymentID != "dep-cheap" {
		t.Errorf("entries[2] = %q, want dep-cheap", entries[2].DeploymentID)
	}
}

func TestBuildDeploymentSummary_ZeroRequestsGuard(t *testing.T) {
	// Deployment with no traces — cost_per_request and tok_per_request must be 0, not +Inf.
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "idle-agent", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 0, TotalCost: 0},
		}},
	}

	entries := buildDeploymentSummary(metrics, nil, nil, nil)
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

func TestBuildDeploymentSummary_TopModel(t *testing.T) {
	metrics := []deploymentMetrics{
		{
			DeploymentID: "dep-1",
			AgentName:    "multi-model-agent",
			DailyMetrics: []langfuse.DailyMetric{
				{CountTraces: 10, TotalCost: 6.0,
					Usage: []langfuse.DailyMetricUsage{
						{Model: "claude-haiku", TotalCost: 1.0},
						{Model: "claude-sonnet", TotalCost: 5.0}, // highest
					}},
			},
		},
	}

	entries := buildDeploymentSummary(metrics, nil, nil, nil)
	if entries[0].TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want claude-sonnet", entries[0].TopModel)
	}
}

func TestBuildDeploymentSummary_CostPerRequestRounding(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "agent-a", DailyMetrics: []langfuse.DailyMetric{
			{CountTraces: 3, TotalCost: 1.0},
		}},
	}

	entries := buildDeploymentSummary(metrics, nil, nil, nil)
	// 1.0 / 3 = 0.3333... → rounded to 4dp = 0.3333
	if entries[0].CostPerRequest != 0.3333 {
		t.Errorf("cost_per_request = %v, want 0.3333", entries[0].CostPerRequest)
	}
}

func TestBuildDeploymentSummary_UsersUsedInversion(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "code-reviewer", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
		{DeploymentID: "dep-2", AgentName: "summarizer", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
	}
	// Two users on dep-1 (alice, bob); only alice on dep-2; one row carries
	// the SDK "-" sentinel which normalizeUserID collapses to "" → dropped.
	// A row against an unknown deployment is skipped (defensive — shouldn't
	// happen since the Q_tags filter is bounded to visibleTagValues). The
	// dave row arrives as a JSON-array tag value — tagStrings() handles
	// both shapes.
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": "deployment:dep-1"},
		{"userId": "u_bob", "tags": "deployment:dep-1"},
		{"userId": "u_alice", "tags": "deployment:dep-2"},
		{"userId": "-", "tags": "deployment:dep-1"},
		{"userId": "u_carol", "tags": "deployment:dep-unknown"},
		{"userId": "u_dave", "tags": []any{"deployment:dep-1"}},
	}

	entries := buildDeploymentSummary(metrics, tagsRows, nil, nil)

	byID := make(map[string][]string)
	for _, e := range entries {
		byID[e.DeploymentID] = e.UsersUsed
	}
	if got, want := byID["dep-1"], []string{"u_alice", "u_bob", "u_dave"}; !equalStrings(got, want) {
		t.Errorf("dep-1 users_used = %v, want %v", got, want)
	}
	if got, want := byID["dep-2"], []string{"u_alice"}; !equalStrings(got, want) {
		t.Errorf("dep-2 users_used = %v, want %v", got, want)
	}
	// dep-unknown row should NOT appear under any visible deployment.
	for id, users := range byID {
		for _, u := range users {
			if u == "u_carol" {
				t.Errorf("u_carol leaked into %s — should have been dropped (unknown deployment)", id)
			}
		}
	}
}

// Linked Slack and WorkOS rows for the same human bucket together
// because the compute path translates the Slack user_id to its WorkOS
// id via applyLinkedSlackUserIDTranslation BEFORE buildDeploymentUserRows
// runs. No merge step downstream — the bucketing is automatic.
func TestBuildDeploymentSummary_TranslatedLinkedRowsBucketByWorkOSId(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "code-reviewer", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
	}
	tagsRows := []map[string]any{
		{"userId": "user_sohum", "tags": "deployment:dep-1"},
		{"userId": "U07SOHUM1", "tags": []any{"deployment:dep-1"}},
	}
	linkMap := map[string]string{
		"U07SOHUM1": "user_sohum",
	}

	applyLinkedSlackUserIDTranslation(linkMap, tagsRows)
	out := buildDeploymentSummaryWithUsers(metrics, buildDeploymentUserRows(tagsRows), nil, nil)
	if len(out) != 1 {
		t.Fatalf("expected one deployment, got %d", len(out))
	}
	if got, want := out[0].UsersUsed, []string{"user_sohum"}; !equalStrings(got, want) {
		t.Fatalf("users_used = %v, want %v", got, want)
	}
	if len(out[0].UsersUsedDetails) != 1 {
		t.Fatalf("expected one identity after translation, got %+v", out[0].UsersUsedDetails)
	}
	if got := out[0].UsersUsedDetails[0]; got.UserID != "user_sohum" || got.UserDetails.Kind != UserDetailsKindAstro {
		t.Errorf("translated identity mismatch: %+v", got)
	}
}

// Unlinked bare-Slack rows pass through compute time unchanged. Profile
// + workspace metadata is stamped by ResolveDeploymentsSummaryIdentities
// at read time — covered by the trace identity tests in
// users_summary_merge_test.go (same stampSlackDirectoryEntry helper).
// Here we just pin that the build layer carries the bare Slack id
// forward without inventing identity data.
func TestBuildDeploymentSummary_UnlinkedSlackRowsPassThroughRaw(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "code-reviewer", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
	}
	tagsRows := []map[string]any{
		{"userId": "U07SOHUM1", "tags": []any{"deployment:dep-1"}},
	}

	out := buildDeploymentSummaryWithUsers(metrics, buildDeploymentUserRows(tagsRows), nil, nil)
	if len(out) != 1 {
		t.Fatalf("expected one deployment, got %d", len(out))
	}
	if len(out[0].UsersUsedDetails) != 1 {
		t.Fatalf("expected one identity, got %+v", out[0].UsersUsedDetails)
	}
	got := out[0].UsersUsedDetails[0]
	if got.UserID != "U07SOHUM1" || got.UserDetails.Kind != UserDetailsKindSlack {
		t.Errorf("unlinked Slack row should classify as kind=slack from id alone, got %+v", got)
	}
	if got.UserDetails.TeamID != "" || got.UserDetails.DisplayName != "" {
		t.Errorf("build layer should not stamp directory data, got %+v", got.UserDetails)
	}
}

func TestBuildDeploymentSummary_SameUserAcrossDeployments(t *testing.T) {
	// Same user touching two deployments of the same agent_name shows up
	// under BOTH deployment rows. No rollup → no dedupe across deployments.
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-east", AgentName: "shared-agent", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
		{DeploymentID: "dep-west", AgentName: "shared-agent", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
	}
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": "deployment:dep-east"},
		{"userId": "u_alice", "tags": []any{"deployment:dep-west"}},
		{"userId": "u_bob", "tags": "deployment:dep-west"},
	}

	entries := buildDeploymentSummary(metrics, tagsRows, nil, nil)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (per deployment), got %d", len(entries))
	}
	byID := make(map[string][]string)
	for _, e := range entries {
		byID[e.DeploymentID] = e.UsersUsed
	}
	if got, want := byID["dep-east"], []string{"u_alice"}; !equalStrings(got, want) {
		t.Errorf("dep-east users_used = %v, want %v", got, want)
	}
	if got, want := byID["dep-west"], []string{"u_alice", "u_bob"}; !equalStrings(got, want) {
		t.Errorf("dep-west users_used = %v, want %v", got, want)
	}
}

// ── discoverTombstoneIDs ──────────────────────────────────────────────────────

func TestDiscoverTombstoneIDs(t *testing.T) {
	live := []*deploymentstore.Deployment{
		{ID: "dep-live-1"},
		{ID: "dep-live-2"},
	}
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": "deployment:dep-live-1"},            // live → skip
		{"userId": "u_bob", "tags": "deployment:dep-archived-a"},          // archived → keep
		{"userId": "u_carol", "tags": []any{"deployment:dep-archived-b"}}, // archived via JSON array → keep
		{"userId": "u_dave", "tags": "deployment:dep-archived-a"},         // duplicate archived → dedupe
		{"userId": "u_eve", "tags": "env:prod"},                           // non-deployment tag → skip
		{"userId": "u_frank", "tags": "deployment:dep-live-2"},            // live → skip
	}

	ids := discoverTombstoneIDs(tagsRows, live)
	sort.Strings(ids)
	if len(ids) != 2 {
		t.Fatalf("expected 2 tombstones, got %d (%v)", len(ids), ids)
	}
	if ids[0] != "dep-archived-a" || ids[1] != "dep-archived-b" {
		t.Errorf("unexpected tombstone ids: %v", ids)
	}
}

// ── buildDeploymentSummary with archivedIDs ───────────────────────────────────

func TestBuildDeploymentSummary_ArchivedIDsMarksEntries(t *testing.T) {
	undeployed := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-live", AgentName: "agent-a", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 10, TotalCost: 5.0}}},
		{DeploymentID: "dep-arch", AgentName: "agent-b", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 4, TotalCost: 2.0}}},
	}
	deployments := []*deploymentstore.Deployment{
		{ID: "dep-live", AgentName: "agent-a"},
		{ID: "dep-arch", AgentName: "agent-b", UndeployedAt: &undeployed},
	}
	archivedIDs := map[string]struct{}{"dep-arch": {}}

	entries := buildDeploymentSummary(metrics, nil, deployments, archivedIDs)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	byID := make(map[string]DeploymentSummaryEntry, len(entries))
	for _, e := range entries {
		byID[e.DeploymentID] = e
	}
	if byID["dep-live"].IsArchived {
		t.Errorf("dep-live: IsArchived = true, want false")
	}
	if !byID["dep-arch"].IsArchived {
		t.Errorf("dep-arch: IsArchived = false, want true")
	}
	if byID["dep-arch"].UndeployedAt == nil || !byID["dep-arch"].UndeployedAt.Equal(undeployed) {
		t.Errorf("dep-arch: UndeployedAt = %v, want %v", byID["dep-arch"].UndeployedAt, undeployed)
	}
}

func TestBuildDeploymentSummary_DropsZeroSpendArchivedEntries(t *testing.T) {
	// Archived deployments with no spend in the window are noise — buildDeploymentSummary
	// drops them so the table doesn't bloat with empty tombstones. Live deployments at
	// zero spend still surface (configured-but-unused is meaningful signal).
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-live-zero", AgentName: "agent-quiet", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 0, TotalCost: 0}}},
		{DeploymentID: "dep-arch-zero", AgentName: "agent-gone", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 0, TotalCost: 0}}},
		{DeploymentID: "dep-arch-spend", AgentName: "agent-gone-but-spendy", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 3, TotalCost: 1.0}}},
	}
	archivedIDs := map[string]struct{}{
		"dep-arch-zero":  {},
		"dep-arch-spend": {},
	}

	entries := buildDeploymentSummary(metrics, nil, nil, archivedIDs)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (live-zero + arch-spend), got %d", len(entries))
	}
	for _, e := range entries {
		if e.DeploymentID == "dep-arch-zero" {
			t.Errorf("dep-arch-zero should have been dropped — archived with zero spend")
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── GetAccountDeploymentsSummary handler ──────────────────────────────────────

func TestGetAccountDeploymentsSummary_NotConfigured(t *testing.T) {
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
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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
	router.GET("/api/v1/accounts/:account/observability/deployments-summary",
		GetAccountDeploymentsSummary(log, cfg, accountStore, nil, langfuseStore, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/deployments-summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AccountDeploymentsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Deployments == nil {
		t.Error("deployments should be empty slice, not nil")
	}
	if len(resp.Deployments) != 0 {
		t.Errorf("deployments len = %d, want 0", len(resp.Deployments))
	}
}

func TestGetAccountDeploymentsSummary_InvalidPeriod(t *testing.T) {
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
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/deployments-summary",
		GetAccountDeploymentsSummary(log, cfg, accountStore, nil, langfuseStore, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/deployments-summary?from=not-a-date&to=also-not-a-date", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid from+to, got %d", rec.Code)
	}
}

func TestGetAccountDeploymentsSummary_HappyPath(t *testing.T) {
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
			// Batched P95 query groups by tags — emit a row per deployment tag.
			json.NewEncoder(w).Encode(langfuse.MetricsResponse{ //nolint:errcheck
				Data: []map[string]any{{"tags": "deployment:dep-1", "p95_latency": 1200.0}},
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
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "encrypted_data_key", "nonce", "created_at"}).
			AddRow("acct-1", "proj-1", "pk", "sk", nil, nil, now))

	depCols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors",
	}
	depMock.ExpectQuery("SELECT .+ FROM deployments").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(depCols).
			AddRow("dep-1", "acct-1", nil, "code-reviewer", "b1", "ns-1", "Code Reviewer",
				"{}", nil, nil, nil, "Running", nil, nil, now, nil, now, nil, nil))

	cfg := &config.Config{}
	cfg.Deployment.LangfuseBaseURL = langfuseSrv.URL

	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/deployments-summary",
		GetAccountDeploymentsSummary(log, cfg, accountStore, depStore, langfuseStore, nil, nil))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/accounts/myorg/observability/deployments-summary?from=2026-05-01T00:00:00Z&to=2026-05-08T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AccountDeploymentsSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d: %s", len(resp.Deployments), rec.Body.String())
	}
	d := resp.Deployments[0]
	if d.DeploymentID != "dep-1" {
		t.Errorf("deployment_id = %q, want dep-1", d.DeploymentID)
	}
	if d.AgentName != "code-reviewer" {
		t.Errorf("agent_name = %q, want code-reviewer", d.AgentName)
	}
	if d.DisplayName != "Code Reviewer" {
		t.Errorf("display_name = %q, want Code Reviewer", d.DisplayName)
	}
	if d.Namespace != "ns-1" {
		t.Errorf("namespace = %q, want ns-1", d.Namespace)
	}
	if d.Requests != 10 {
		t.Errorf("requests = %d, want 10", d.Requests)
	}
	if d.P95LatencyMs != 1200 {
		t.Errorf("p95_latency_ms = %d, want 1200", d.P95LatencyMs)
	}
	if d.TopModel != "claude-sonnet" {
		t.Errorf("top_model = %q, want claude-sonnet", d.TopModel)
	}
	if resp.Period.Start != "2026-05-01T00:00:00Z" {
		t.Errorf("period.start = %q, want 2026-05-01T00:00:00Z", resp.Period.Start)
	}
}
