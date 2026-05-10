package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// ---------------------------------------------------------------------------
// Pure projection helpers
// ---------------------------------------------------------------------------

func TestTraceHasDeploymentTag(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		dep  string
		want bool
	}{
		{"matches", []string{"env:prod", "deployment:dep-1"}, "dep-1", true},
		{"no match", []string{"deployment:other"}, "dep-1", false},
		{"empty tags", nil, "dep-1", false},
		{"prefix only", []string{"deployment:"}, "dep-1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := traceHasDeploymentTag(c.tags, c.dep); got != c.want {
				t.Errorf("traceHasDeploymentTag(%v,%q) = %v, want %v", c.tags, c.dep, got, c.want)
			}
		})
	}
}

func TestProjectTrace_LatencyConvertedSecondsToMs(t *testing.T) {
	d := &langfuse.TraceDetail{
		Trace: langfuse.Trace{
			ID:        "t-1",
			Name:      "root",
			Latency:   2.49, // seconds
			TotalCost: 0.0068,
			CreatedAt: "2026-05-09T12:00:00Z",
			Tags:      []string{"deployment:dep-1"},
		},
		UserID:      "u-1",
		Environment: "prod",
	}
	out := projectTrace(d)
	if got := out["latency_ms"]; got != 2490.0 {
		t.Errorf("latency_ms = %v, want 2490", got)
	}
	if out["trace_id"] != "t-1" {
		t.Errorf("trace_id = %v, want t-1", out["trace_id"])
	}
	if out["user_id"] != "u-1" {
		t.Errorf("user_id = %v, want u-1", out["user_id"])
	}
}

func TestProjectObservations_LatencyConvertedAndShape(t *testing.T) {
	obs := []langfuse.Observation{
		{
			ID:                  "o-1",
			TraceID:             "t-1",
			ParentObservationID: "",
			Type:                "GENERATION",
			Name:                "chat",
			StartTime:           "2026-05-09T12:00:00Z",
			EndTime:             "2026-05-09T12:00:02Z",
			Latency:             2.23, // seconds — must convert to 2230ms
			Model:               "claude-sonnet-4",
			ModelParameters:     map[string]any{"temperature": 0.0},
			Level:               "DEFAULT",
			CalculatedTotalCost: 0.0068,
			Usage:               &langfuse.Usage{Input: 2139, Output: 25, Total: 2164, Unit: "TOKENS"},
		},
		{
			ID:                  "o-2",
			TraceID:             "t-1",
			ParentObservationID: "o-1",
			Type:                "SPAN",
			Name:                "model_step",
			StartTime:           "2026-05-09T12:00:00Z",
			Latency:             2.17,
			Level:               "ERROR",
			StatusMessage:       "boom",
		},
	}
	out := projectObservations(obs)
	if len(out) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(out))
	}

	// First: generation with full payload
	g := out[0]
	if g["latency_ms"] != 2230.0 {
		t.Errorf("o-1 latency_ms = %v, want 2230", g["latency_ms"])
	}
	if g["type"] != "generation" {
		t.Errorf("o-1 type = %v, want generation (lowercased)", g["type"])
	}
	if g["model"] != "claude-sonnet-4" {
		t.Errorf("o-1 model = %v, want claude-sonnet-4", g["model"])
	}
	if g["level"] != "default" {
		t.Errorf("o-1 level = %v, want default", g["level"])
	}
	usage, ok := g["usage"].(gin.H)
	if !ok {
		t.Fatalf("o-1 usage not gin.H: %T", g["usage"])
	}
	if usage["input"] != 2139 || usage["output"] != 25 || usage["total"] != 2164 {
		t.Errorf("o-1 usage = %+v, want 2139/25/2164", usage)
	}

	// Second: span with no model / usage; status_message + error level surface
	s := out[1]
	if s["latency_ms"] != 2170.0 {
		t.Errorf("o-2 latency_ms = %v, want 2170", s["latency_ms"])
	}
	if s["type"] != "span" {
		t.Errorf("o-2 type = %v, want span", s["type"])
	}
	if _, hasModel := s["model"]; hasModel {
		t.Errorf("o-2 should not have model key when empty")
	}
	if _, hasUsage := s["usage"]; hasUsage {
		t.Errorf("o-2 should not have usage when nil")
	}
	if s["status_message"] != "boom" {
		t.Errorf("o-2 status_message = %v", s["status_message"])
	}
	if s["level"] != "error" {
		t.Errorf("o-2 level = %v, want error", s["level"])
	}
}

func TestProjectScores_LowerCasesDataType(t *testing.T) {
	scores := []langfuse.Score{
		{ID: "s-1", Name: "helpfulness", Value: 0.8, DataType: "NUMERIC", Source: "API"},
		{ID: "s-2", Name: "tone", StringValue: "polite", DataType: "CATEGORICAL"},
	}
	out := projectScores(scores)
	if len(out) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(out))
	}
	if out[0]["data_type"] != "numeric" {
		t.Errorf("data_type = %v, want numeric", out[0]["data_type"])
	}
	if out[1]["string_value"] != "polite" {
		t.Errorf("string_value = %v, want polite", out[1]["string_value"])
	}
}

// ---------------------------------------------------------------------------
// Handler integration — uses sqlmock + a stub Langfuse server
// ---------------------------------------------------------------------------

type traceDetailFixture struct {
	router       *gin.Engine
	accountMock  sqlmock.Sqlmock
	langfuseMock sqlmock.Sqlmock
	deployMock   sqlmock.Sqlmock
	upstream     *httptest.Server
}

// setupTraceDetailRouter wires up everything the handler needs:
// auth (user injected on context), DBs (account + langfuse_creds + deployments),
// and an upstream Langfuse server whose response the test controls.
func setupTraceDetailRouter(t *testing.T, withUser bool, upstreamHandler http.HandlerFunc) *traceDetailFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, langfuseMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	langfuseStore := langfuse.NewStore(langfuseDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{Deployment: config.DeploymentConfig{LangfuseBaseURL: upstream.URL}}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if withUser {
			c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		}
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/observability/traces/:traceId",
		GetLangfuseTraceDetail(log, cfg, accountStore, deployStore, langfuseStore))

	return &traceDetailFixture{
		router:       router,
		accountMock:  accountMock,
		langfuseMock: langfuseMock,
		deployMock:   deployMock,
		upstream:     upstream,
	}
}

// expectAuthorizedDeployment seeds the standard chain of DB queries that
// resolveLangfuseContext makes: deployment lookup → membership check →
// langfuse credentials lookup.
func expectAuthorizedDeployment(f *traceDetailFixture) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "sasbot", "build-1", "ns-1")
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	f.langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key",
			"encrypted_data_key", "nonce", "created_at",
		}).AddRow("acct-1", "proj-1", "pk-lf", "sk-lf", []byte{}, []byte{}, time.Now()))
}

func TestGetLangfuseTraceDetail_OK(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
			Trace: langfuse.Trace{
				ID:        "trace-x",
				Name:      "invoke_agent",
				Latency:   2.49,
				TotalCost: 0.0068,
				CreatedAt: "2026-05-09T12:00:00Z",
				Tags:      []string{"deployment:dep-1"},
			},
			Observations: []langfuse.Observation{
				{ID: "o-1", Type: "SPAN", Name: "root", Latency: 2.49, StartTime: "2026-05-09T12:00:00Z"},
			},
			Scores: []langfuse.Score{},
		})
	}
	f := setupTraceDetailRouter(t, true, upstream)
	expectAuthorizedDeployment(f)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/observability/traces/trace-x", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Trace        map[string]any   `json:"trace"`
		Observations []map[string]any `json:"observations"`
		Scores       []map[string]any `json:"scores"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Trace["latency_ms"] != 2490.0 {
		t.Errorf("trace.latency_ms = %v, want 2490 (seconds → ms)", resp.Trace["latency_ms"])
	}
	if len(resp.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(resp.Observations))
	}
	if resp.Observations[0]["latency_ms"] != 2490.0 {
		t.Errorf("obs[0].latency_ms = %v, want 2490", resp.Observations[0]["latency_ms"])
	}
}

func TestGetLangfuseTraceDetail_Upstream404IsClient404(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"trace not found"}`, http.StatusNotFound)
	}
	f := setupTraceDetailRouter(t, true, upstream)
	expectAuthorizedDeployment(f)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/observability/traces/missing", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (not 502), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetLangfuseTraceDetail_Upstream500IsBadGateway(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `boom`, http.StatusInternalServerError)
	}
	f := setupTraceDetailRouter(t, true, upstream)
	expectAuthorizedDeployment(f)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/observability/traces/trace-x", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A trace from another deployment (or with no deployment tag) must be 404'd
// rather than leaked through, even though Langfuse itself returned it
// successfully — defense in depth around tenant isolation.
func TestGetLangfuseTraceDetail_TagMismatchIs404(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
			Trace: langfuse.Trace{
				ID:        "trace-x",
				Name:      "leak-attempt",
				CreatedAt: "2026-05-09T12:00:00Z",
				Tags:      []string{"deployment:other-dep"}, // wrong dep
			},
		})
	}
	f := setupTraceDetailRouter(t, true, upstream)
	expectAuthorizedDeployment(f)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/observability/traces/trace-x", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-deployment trace, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetLangfuseTraceDetail_Unauthorized(t *testing.T) {
	f := setupTraceDetailRouter(t, false, func(_ http.ResponseWriter, _ *http.Request) {
		t.Errorf("upstream should not be called when unauthorized")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/observability/traces/trace-x", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}
