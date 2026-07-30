package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/gin-gonic/gin"
)

// --- Pure function tests ---

func TestIsSafeRedirectPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"", false},
		{"/knowledge/new", true},
		{"/knowledge/new?provider=supabase", true},
		{"//evil.com", false},
		{"https://evil.com", false},
		{"/path/with@at", false},
		{"relative/no/slash", false},
	}
	for _, tc := range cases {
		if got := isSafeRedirectPath(tc.path); got != tc.want {
			t.Errorf("isSafeRedirectPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestAppendParam(t *testing.T) {
	if got := appendParam("/settings", "x=1"); got != "/settings?x=1" {
		t.Errorf("no query: got %q", got)
	}
	if got := appendParam("/knowledge/new?provider=supabase", "x=1"); got != "/knowledge/new?provider=supabase&x=1" {
		t.Errorf("existing query: got %q", got)
	}
}

// installSupabaseStub swaps http.DefaultTransport so every outbound request is
// served by a fake WorkOS Pipes + Supabase API. tokenStatus controls the WorkOS
// token endpoint; projectsStatus/projectsBody control GET /v1/projects.
func installSupabaseStub(t *testing.T, tokenStatus, projectsStatus int, projectsBody string) func() {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/supabase/token", func(w http.ResponseWriter, _ *http.Request) {
		if tokenStatus != http.StatusOK {
			w.WriteHeader(tokenStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true,
			"access_token": map[string]any{
				"access_token": "sb-access-token",
				"scopes":       []string{},
			},
		})
	})
	mux.HandleFunc("/user_management/users/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1/projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(projectsStatus)
		_, _ = w.Write([]byte(projectsBody))
	})
	// Subtree — serves /v1/projects/<ref>/health with a canned healthy response.
	mux.HandleFunc("/v1/projects/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"db","status":"ACTIVE_HEALTHY","healthy":true}]`))
	})

	srv := httptest.NewServer(mux)
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	return func() {
		http.DefaultTransport = old
		srv.Close()
	}
}

func TestSupabaseStatus_Connected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, "[]")
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/status", SupabaseStatus(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != true {
		t.Errorf("expected connected: true, got %v", resp)
	}
}

func TestSupabaseStatus_NotConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// WorkOS token endpoint errors → GetAccessToken fails → connected:false.
	restore := installSupabaseStub(t, http.StatusInternalServerError, http.StatusOK, "[]")
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/status", SupabaseStatus(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != false {
		t.Errorf("expected connected: false, got %v", resp)
	}
}

func TestSupabaseListProjects_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `[{"id":"abc","name":"My Project","region":"us-east-1","organization_id":"org-x"}]`
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, body)
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/projects", SupabaseListProjects(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Projects []SupabaseProject `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Projects) != 1 || resp.Projects[0].ID != "abc" || resp.Projects[0].OrgID != "org-x" {
		t.Errorf("unexpected projects: %+v", resp.Projects)
	}
}

func TestSupabaseListProjects_RevokedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// WorkOS serves a token, but Supabase rejects it (401) → 422 not_connected.
	restore := installSupabaseStub(t, http.StatusOK, http.StatusUnauthorized, "unauthorized")
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/projects", SupabaseListProjects(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "supabase_not_connected" {
		t.Errorf("expected supabase_not_connected, got %v", resp)
	}
}

func TestSupabaseAPICallsSetAffiliateUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/supabase/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":       true,
			"access_token": map[string]any{"access_token": "sb-access-token", "scopes": []string{}},
		})
	})
	mux.HandleFunc("/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	srv := httptest.NewServer(mux)
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	defer func() {
		http.DefaultTransport = old
		srv.Close()
	}()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/projects", SupabaseListProjects(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/projects", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	if gotUA != "Astro AI" {
		t.Errorf("expected Supabase API call User-Agent %q, got %q", "Astro AI", gotUA)
	}
}

func TestSupabaseProjectHealth_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, "[]")
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/projects/:ref/health", SupabaseProjectHealth(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/projects/abcdefghijklmnop/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// The provider payload is proxied verbatim under "services".
	if !strings.Contains(rec.Body.String(), "ACTIVE_HEALTHY") || !strings.Contains(rec.Body.String(), `"services"`) {
		t.Errorf("unexpected health body: %s", rec.Body.String())
	}
}

func TestSupabaseProjectHealth_InvalidRef(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/projects/:ref/health", SupabaseProjectHealth(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/projects/bad!ref/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid ref, got %d", rec.Code)
	}
}

func TestSupabaseDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, "[]")
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.DELETE("/api/v1/accounts/:account/supabase", SupabaseDisconnect(logger.New("error", "json"), pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount/supabase", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["disconnected"] != true {
		t.Errorf("expected disconnected: true, got %v", resp)
	}
}

func TestSupabaseCallback_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, "[]")
	defer restore()

	cfg := SupabaseHandlerConfig{FrontendURL: "https://app.example.com"}
	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/supabase/callback", SupabaseCallback(logger.New("error", "json"), pipes.New("fake-key"), cfg))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/callback?redirect_to=/settings/connectors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/settings/connectors") || !strings.Contains(loc, "supabase_connected=true") {
		t.Errorf("unexpected redirect: %s", loc)
	}
}

func TestSupabaseCallback_NotAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := SupabaseHandlerConfig{FrontendURL: "https://app.example.com"}
	router := gin.New() // no session middleware
	router.GET("/api/v1/accounts/:account/supabase/callback", SupabaseCallback(logger.New("error", "json"), pipes.New("fake-key"), cfg))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/supabase/callback?redirect_to=/settings/connectors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "supabase_error=not_authenticated") {
		t.Errorf("expected not_authenticated error redirect, got %s", rec.Header().Get("Location"))
	}
}

func TestSupabaseConnect_AlreadyConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := installSupabaseStub(t, http.StatusOK, http.StatusOK, "[]")
	defer restore()

	cfg := SupabaseHandlerConfig{WebhookBaseURL: "https://api.example.com", FrontendURL: "https://app.example.com"}
	router := gin.New()
	router.Use(injectTestSession(), injectTestAccount())
	router.POST("/api/v1/accounts/:account/supabase/connect", SupabaseConnect(logger.New("error", "json"), pipes.New("fake-key"), cfg))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/testaccount/supabase/connect", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != true {
		t.Errorf("expected connected: true, got %v", resp)
	}
}
