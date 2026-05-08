package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/gin-gonic/gin"
)

// injectTestSession is a test middleware that sets a fake session in context,
// simulating what RequireAuth middleware does in production.
func injectTestSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			ID:             "test-session-id",
			UserID:         "user-1",
			OrganizationID: "org-1",
		})
		c.Next()
	}
}

// makeHMAC computes the sha256= prefixed HMAC-SHA256 signature for body using secret.
func makeHMAC(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// --- Pure function tests ---

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "test-webhook-secret"
	validSig := makeHMAC(body, secret)

	tests := []struct {
		name string
		sig  string
		want bool
	}{
		{"valid signature", validSig, true},
		{"wrong signature", "sha256=" + strings.Repeat("a", 64), false},
		{"empty signature", "", false},
		{"no sha256= prefix", hex.EncodeToString([]byte("notprefixed")), false},
		{"wrong prefix", "sha1=abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyGitHubSignature(body, secret, tt.sig)
			if got != tt.want {
				t.Errorf("verifyGitHubSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstCommitLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single line", "fix: update config", "fix: update config"},
		{"multi-line returns first", "feat: add feature\n\nMore details here.", "feat: add feature"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"leading/trailing whitespace on first line", "  subject line  \n\nbody", "subject line"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstCommitLine(tt.input)
			if got != tt.want {
				t.Errorf("firstCommitLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- GitHubWebhook tests ---

func TestGitHubWebhook_NonPushEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, nil, nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "ping")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestGitHubWebhook_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, nil, nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("not json"))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestGitHubWebhook_UnknownRepo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	// webhookStore.Get: no webhook registered for this repo → handler returns 200.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs("owner/unknown-repo").
		WillReturnRows(sqlmock.NewRows(webhookCols()))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, nil))

	payload := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"owner/unknown-repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestGitHubWebhook_InvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	repoFullName := "owner/repo"
	webhookSecret := "correct-secret"

	// webhookStore.Get: returns the global webhook secret for HMAC verification.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs(repoFullName).
		WillReturnRows(webhookRow(repoFullName, webhookSecret, 12345))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, nil))

	payload := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"owner/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64)) // wrong sig
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestGitHubWebhook_WrongBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	repoFullName := "owner/repo"
	webhookSecret := "my-secret"

	// The push is to "feat" but the connection tracks "main".
	payloadBody := `{"ref":"refs/heads/feat","after":"abc123def456","repository":{"full_name":"owner/repo"}}`

	// webhookStore.Get: returns the webhook secret for HMAC verification.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs(repoFullName).
		WillReturnRows(webhookRow(repoFullName, webhookSecret, 12345))

	// ListByRepoAndBranch: no connections for repo+branch "feat" — push is ignored.
	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs(repoFullName, "feat").
		WillReturnRows(sqlmock.NewRows(connCols()))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payloadBody))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", makeHMAC([]byte(payloadBody), webhookSecret))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestGitHubWebhook_BranchDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	repoFullName := "owner/repo"
	webhookSecret := "my-secret"

	// after == zeros means branch deletion.
	payloadBody := `{"ref":"refs/heads/main","after":"0000000000000000000000000000000000000000","repository":{"full_name":"owner/repo"}}`

	// webhookStore.Get: returns the webhook secret for HMAC verification.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs(repoFullName).
		WillReturnRows(webhookRow(repoFullName, webhookSecret, 12345))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payloadBody))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", makeHMAC([]byte(payloadBody), webhookSecret))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// --- Unauthenticated path tests ---

func TestGitHubAccountStatus_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	// No session middleware — GetSession will return false.
	router.GET("/api/v1/accounts/:account/github", GitHubAccountStatus(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "not authenticated" {
		t.Errorf("expected 'not authenticated' error, got %v", resp["error"])
	}
}

// TestGitHubAccountStatus_StaleToken verifies that when WorkOS returns a token but
// GitHub rejects it with 401, the status endpoint returns connected: false so the
// frontend prompts reconnect rather than showing a bare "@" with no login.
func TestGitHubAccountStatus_StaleToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restore := installStaleGitHubTokenStub(t)
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/github", GitHubAccountStatus(logger.New("error", "json"), pipes.New("fake-workos-key")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["connected"] != false {
		t.Errorf("expected connected: false for stale token, got %v", resp["connected"])
	}
	if _, ok := resp["github_login"]; ok {
		t.Errorf("expected no github_login in response, got %v", resp["github_login"])
	}
}

func TestGitHubAccountDisconnect_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	// No session middleware — GetSession will return false.
	router.DELETE("/api/v1/accounts/:account/github", GitHubAccountDisconnect(log, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount/github", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "not authenticated" {
		t.Errorf("expected 'not authenticated' error, got %v", resp["error"])
	}
}

// --- GitHubAccountListConnections tests ---

func TestGitHubAccountListConnections_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	now := time.Now().UTC().Truncate(time.Second)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	store := githubconnection.New(db)

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "created_at"}).
			AddRow("my-agent", "owner/my-repo", now))

	injectAcct := func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testaccount"})
		c.Next()
	}
	router := gin.New()
	router.GET("/api/v1/accounts/:account/github/connections",
		injectAcct,
		GitHubAccountListConnections(log, store),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github/connections", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp struct {
		Connections []struct {
			AgentName    string `json:"agent_name"`
			RepoFullName string `json:"repo_full_name"`
			CreatedAt    string `json:"created_at"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Connections) != 1 {
		t.Fatalf("got %d connections, want 1", len(resp.Connections))
	}
	c := resp.Connections[0]
	if c.AgentName != "my-agent" {
		t.Errorf("agent_name = %q, want %q", c.AgentName, "my-agent")
	}
	if c.RepoFullName != "owner/my-repo" {
		t.Errorf("repo_full_name = %q, want %q", c.RepoFullName, "owner/my-repo")
	}
	if c.CreatedAt == "" {
		t.Errorf("created_at is empty")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestGitHubAccountListConnections_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	// No account middleware — GetAccountFromContext will return false.
	router.GET("/api/v1/accounts/:account/github/connections", GitHubAccountListConnections(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github/connections", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}

// --- shared external API stub -------------------------------------------
//
// Swaps http.DefaultTransport so outbound HTTPS calls from the WorkOS SDK,
// pipes.Client, and github.Client are routed to an in-process httptest.Server.
// All three clients use http.Client with no explicit Transport, so they inherit
// http.DefaultTransport.

type externalAPIStub struct {
	workosToken      string
	githubDeleteHits atomic.Int32
	githubPostHits   atomic.Int32
	server           *httptest.Server
}

func installExternalAPIStub(t *testing.T) (*externalAPIStub, func()) {
	t.Helper()
	stub := &externalAPIStub{workosToken: "fake-github-oauth-token"}

	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/github/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true,
			"access_token": map[string]any{
				"access_token": stub.workosToken,
				"scopes":       []string{"repo"},
			},
		})
	})
	mux.HandleFunc("/user_management/users/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/hooks/"):
			stub.githubDeleteHits.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/hooks"):
			stub.githubPostHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 99, "active": true, "events": []string{"push"}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			// PathExists check — return 200 to indicate the subpath exists.
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	stub.server = srv

	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	return stub, func() {
		http.DefaultTransport = old
		srv.Close()
	}
}

type rewriteTransport struct{ server *httptest.Server }

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.server.URL, "http://")
	clone.Host = clone.URL.Host
	clone.RequestURI = ""
	return r.server.Client().Do(clone)
}

// TestGitHubAccountDisconnect_KeepsWebhookWhenOtherSubpathConnExists verifies that when
// one agent is disconnected, the shared webhook is preserved if the same account still
// has other connections to the same base repo (e.g. subpath connections).
func TestGitHubAccountDisconnect_KeepsWebhookWhenOtherSubpathConnExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("acct-A").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "created_at"}).
			AddRow("agent-a", "owner/repo", time.Now()))

	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-A", "agent-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections: another connection still references owner/repo — no rows returned.
	mock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}))

	pipesClient := pipes.New("fake-workos-key")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-A", OrganizationID: "org-A"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-A", Name: "accountA"})
		c.Next()
	})
	router.DELETE("/accounts/:account/github",
		GitHubAccountDisconnect(logger.New("error", "json"), pipesClient, store, whStore, k8scache.NoopCache{}))

	req := httptest.NewRequest(http.MethodDelete, "/accounts/accountA/github", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if stub.githubDeleteHits.Load() != 0 {
		t.Errorf("expected 0 webhook DELETEs when another account still references it; got %d",
			stub.githubDeleteHits.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// TestGitHubLink_ReusesWebhookWithinSameAccount verifies that when an account adds a
// subpath connection to a repo it already has a webhook for, the existing webhook is
// reused — no new webhook is created on GitHub.
func TestGitHubLink_ReusesWebhookWithinSameAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	mock.MatchExpectationsInOrder(false)

	// Get(acct-A, agent-svc) → no existing connection.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND agent_name = \$2`).
		WithArgs("acct-A", "agent-svc").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// GetByRepoForAccount(acct-A, "owner/repo/svc") → no conflict.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND repo_full_name = \$2`).
		WithArgs("acct-A", "owner/repo/svc").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// Upsert — 7 args, no webhook_id/secret.
	mock.ExpectExec("INSERT INTO github_connections").
		WithArgs("acct-A", "accountA", "agent-svc", "user-1", "org-1", "owner/repo/svc", "main").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// webhookStore.Get("owner/repo") → finds existing webhook — reuse, no new webhook created.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(webhookRow("owner/repo", "shared-secret", 99))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-A", Name: "accountA"})
		c.Next()
	})
	router.POST("/agents/:account/:name/github/link",
		GitHubLink(logger.New("error", "json"), pipes.New("fake"), store, whStore,
			GitHubHandlerConfig{WebhookBaseURL: "https://api.astropods.ai"}))

	req := httptest.NewRequest(http.MethodPost, "/agents/accountA/agent-svc/github/link",
		strings.NewReader(`{"repo_full_name":"owner/repo/svc","branch":"main"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if stub.githubPostHits.Load() != 0 {
		t.Errorf("expected 0 POST /hooks calls (webhook reused); got %d", stub.githubPostHits.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// TestValidateRepoFullName_RejectsMetacharacters verifies that characters outside
// [A-Za-z0-9._-/] are rejected. These characters are unsafe because repo_full_name
// flows into GitHub API URLs and git-clone init container commands.
func TestValidateRepoFullName_RejectsMetacharacters(t *testing.T) {
	bad := []string{
		"owner/repo; rm -rf /",
		"owner/$(curl attacker)/pwn",
		"owner/repo`id`",
		"owner/repo?ref=main",
		"owner/repo#frag",
		"owner/repo\npath",
		"owner/re po",
		"owner/repo\x00",
		"owner/repo&cmd",
		"owner/re|po",
	}
	for _, input := range bad {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			if err := validateRepoFullName(input); err == nil {
				t.Errorf("validateRepoFullName(%q) should reject characters outside [A-Za-z0-9._-/]", input)
			}
		})
	}
}

// TestValidateRepoFullName_RejectsOversizedSegments verifies that segments exceeding
// GitHub's 100-character limit are rejected.
func TestValidateRepoFullName_RejectsOversizedSegments(t *testing.T) {
	const maxSegmentLen = 100
	huge := strings.Repeat("a", maxSegmentLen+1)
	input := "owner/" + huge
	if err := validateRepoFullName(input); err == nil {
		t.Errorf("validateRepoFullName accepted a %d-char segment; GitHub caps names at %d", len(huge), maxSegmentLen)
	}
}

// TestArchiveAgent_KeepsWebhookWhenSubpathConnExists verifies that archiving an agent whose
// repo is a subpath does not delete the shared webhook when another agent on the same
// account still references the same base repo.
func TestArchiveAgent_KeepsWebhookWhenSubpathConnExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	indexDB, indexMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New (index): %v", err)
	}
	defer indexDB.Close()

	ghDB, ghMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New (gh): %v", err)
	}
	defer ghDB.Close()

	index := agentindex.NewIndexWithDB(indexDB)
	store := githubconnection.New(ghDB)
	whStore := githubwebhook.New(ghDB)

	indexMock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Get: returns service-a's connection on the subpath.
	ghMock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("test-account-id", "service-a").
		WillReturnRows(connRow("conn-a", "test-account-id", "testaccount", "service-a",
			"owner/repo/service-a", "main"))

	ghMock.ExpectExec("DELETE FROM github_connections").
		WithArgs("test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections: another connection still references owner/repo — no rows returned.
	ghMock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}))

	router := gin.New()
	router.Use(injectTestAccount(), injectTestSession())
	router.POST("/agents/:account/:name/archive",
		ArchiveAgent(logger.New("error", "json"), index, nil, nil, nil, store, whStore, pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/service-a/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	time.Sleep(100 * time.Millisecond) // let the background goroutine complete

	if stub.githubDeleteHits.Load() != 0 {
		t.Errorf("expected 0 webhook DELETEs when another subpath connection still exists; got %d",
			stub.githubDeleteHits.Load())
	}
	if err := ghMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet gh SQL expectations: %v", err)
	}
}

// TestArchiveAgent_DeletesWebhookWhenLastConn verifies that archiving the last agent
// connected to a base repo deletes the webhook from GitHub.
func TestArchiveAgent_DeletesWebhookWhenLastConn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	indexDB, indexMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New (index): %v", err)
	}
	defer indexDB.Close()

	ghDB, ghMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New (gh): %v", err)
	}
	defer ghDB.Close()

	index := agentindex.NewIndexWithDB(indexDB)
	store := githubconnection.New(ghDB)
	whStore := githubwebhook.New(ghDB)

	indexMock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ghMock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("test-account-id", "service-a").
		WillReturnRows(connRow("conn-a", "test-account-id", "testaccount", "service-a",
			"owner/repo/service-a", "main"))

	ghMock.ExpectExec("DELETE FROM github_connections").
		WithArgs("test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections: no remaining connections — returns the webhook_id.
	ghMock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}).AddRow(int64(42)))

	router := gin.New()
	router.Use(injectTestAccount(), injectTestSession())
	router.POST("/agents/:account/:name/archive",
		ArchiveAgent(logger.New("error", "json"), index, nil, nil, nil, store, whStore, pipes.New("fake-key")))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/service-a/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	time.Sleep(100 * time.Millisecond) // let the background goroutine complete

	if stub.githubDeleteHits.Load() != 1 {
		t.Errorf("expected 1 webhook DELETE when last connection is archived; got %d",
			stub.githubDeleteHits.Load())
	}
	if err := ghMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet gh SQL expectations: %v", err)
	}
}

// TestGitHubLink_OrphansWebhookOnPKConflict verifies that when two instances race to
// register a webhook for the same base repo, the loser (INSERT returns 0 rows) deletes
// its orphaned GitHub webhook and still returns 201.
func TestGitHubLink_OrphansWebhookOnPKConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	// No existing connection for this agent.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND agent_name = \$2`).
		WithArgs("acct-1", "agent-a").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// No conflicting connection for this repo.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND repo_full_name = \$2`).
		WithArgs("acct-1", "owner/repo").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// Upsert succeeds.
	mock.ExpectExec("INSERT INTO github_connections").
		WithArgs("acct-1", "testaccount", "agent-a", "user-1", "org-1", "owner/repo", "main").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// webhookStore.Get → no existing webhook, so a new one is created via GitHub API.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows(webhookCols()))

	// webhookStore.Insert → 0 rows affected: another instance won the race.
	mock.ExpectExec("INSERT INTO github_webhooks").
		WithArgs("owner/repo", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testaccount"})
		c.Next()
	})
	router.POST("/agents/:account/:name/github/link",
		GitHubLink(logger.New("error", "json"), pipes.New("fake"), store, whStore,
			GitHubHandlerConfig{WebhookBaseURL: "https://api.astropods.ai"}))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/agent-a/github/link",
		strings.NewReader(`{"repo_full_name":"owner/repo","branch":"main"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if stub.githubPostHits.Load() != 1 {
		t.Errorf("expected 1 POST /hooks; got %d", stub.githubPostHits.Load())
	}
	if stub.githubDeleteHits.Load() != 1 {
		t.Errorf("expected 1 DELETE /hooks to clean up orphaned webhook; got %d", stub.githubDeleteHits.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// installTokenErrorStub swaps http.DefaultTransport with a stub where the WorkOS
// token endpoint returns 500, causing GetAccessToken to return a non-nil error.
// Tracks GitHub DELETE /hooks hits and returns a restore func.
func installTokenErrorStub(t *testing.T) (deleteHits *atomic.Int32, restore func()) {
	t.Helper()
	var hits atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/github/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/user_management/users/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/hooks/") {
			hits.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	srv := httptest.NewServer(mux)
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	return &hits, func() {
		http.DefaultTransport = old
		srv.Close()
	}
}

// TestGitHubLink_InsertDBError_CleansUpOrphanedGitHubWebhook verifies that when
// webhookStore.Insert returns a non-conflict DB error, the GitHub-side webhook that
// was just created is deleted to avoid leaving an orphaned webhook on the repo.
func TestGitHubLink_InsertDBError_CleansUpOrphanedGitHubWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub, restore := installExternalAPIStub(t)
	defer restore()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	// No existing connection for this agent (oldBase stays empty).
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND agent_name = \$2`).
		WithArgs("acct-1", "agent-a").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// No conflicting connection for this repo.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND repo_full_name = \$2`).
		WithArgs("acct-1", "owner/repo").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// Upsert succeeds.
	mock.ExpectExec("INSERT INTO github_connections").
		WithArgs("acct-1", "testaccount", "agent-a", "user-1", "org-1", "owner/repo", "main").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// webhookStore.Get → no existing webhook; new one will be created via GitHub API.
	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows(webhookCols()))

	// webhookStore.Insert → transient DB error. The GitHub webhook was already created;
	// the handler must roll it back.
	mock.ExpectExec("INSERT INTO github_webhooks").
		WithArgs("owner/repo", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("simulated transient db error"))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testaccount"})
		c.Next()
	})
	router.POST("/agents/:account/:name/github/link",
		GitHubLink(logger.New("error", "json"), pipes.New("fake"), store, whStore,
			GitHubHandlerConfig{WebhookBaseURL: "https://api.astropods.ai"}))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/agent-a/github/link",
		strings.NewReader(`{"repo_full_name":"owner/repo","branch":"main"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := stub.githubPostHits.Load(); got != 1 {
		t.Errorf("expected 1 POST /hooks; got %d", got)
	}
	if got := stub.githubDeleteHits.Load(); got != 1 {
		t.Errorf("expected 1 DELETE /hooks to roll back orphaned webhook after insert error; got %d", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// TestGitHubAccountDisconnect_TokenError_DeletesWebhookRow verifies that when the
// account's OAuth token is unavailable, the github_webhooks row is still deleted
// when no connections remain. The GitHub API call is skipped (best-effort), but the
// DB row must always be removed to avoid leaving an orphaned row.
func TestGitHubAccountDisconnect_TokenError_DeletesWebhookRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deleteHits, restore := installTokenErrorStub(t)
	defer restore()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("acct-A").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "created_at"}).
			AddRow("agent-a", "owner/repo", time.Now()))

	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-A", "agent-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections must run even when the token is unavailable.
	mock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}).AddRow(int64(42)))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-A", OrganizationID: "org-A"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-A", Name: "accountA"})
		c.Next()
	})
	router.DELETE("/accounts/:account/github",
		GitHubAccountDisconnect(logger.New("error", "json"), pipes.New("fake-workos-key"), store, whStore, k8scache.NoopCache{}))

	req := httptest.NewRequest(http.MethodDelete, "/accounts/accountA/github", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deleteHits.Load() != 0 {
		t.Errorf("expected 0 GitHub DELETE /hooks when token is unavailable; got %d", deleteHits.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations (DeleteIfNoConnections did not run): %v", err)
	}
}

// installStaleGitHubTokenStub returns a stub where WorkOS hands back a valid token
// but all GitHub API calls respond 401 — simulating a token that WorkOS cached but
// GitHub has already revoked.
func installStaleGitHubTokenStub(t *testing.T) func() {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/github/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true,
			"access_token": map[string]any{
				"access_token": "stale-token",
				"scopes":       []string{"repo"},
			},
		})
	})
	respond401 := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials","status":"401"}`))
	}
	mux.HandleFunc("/user", respond401)
	mux.HandleFunc("/user/", respond401)

	srv := httptest.NewServer(mux)
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	return func() {
		http.DefaultTransport = old
		srv.Close()
	}
}

// mapCache is an in-memory k8scache.Cache for tests.
type mapCache map[string][]byte

func (m mapCache) Get(_ context.Context, key string) ([]byte, bool) {
	v, ok := m[key]
	return v, ok
}
func (m mapCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	m[key] = data
	return nil
}
func (m mapCache) Invalidate(_ context.Context, key string) error {
	delete(m, key)
	return nil
}

// TestGitHubAccountConnect_StaleToken_ReturnsOAuthURL verifies that when WorkOS
// returns a token but GitHub rejects it with 401, the connect endpoint clears the
// stale WorkOS connection and returns a fresh OAuth URL instead of connected: true.
func TestGitHubAccountConnect_StaleToken_ReturnsOAuthURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var deleteHits atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/github/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true,
			"access_token": map[string]any{
				"access_token": "stale-token",
				"scopes":       []string{"repo"},
			},
		})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials","status":"401"}`))
	})
	mux.HandleFunc("/user_management/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteHits.Add(1)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/data-integrations/github/authorize", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://github.com/login/oauth/authorize?fake=1"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	defer func() { http.DefaultTransport = old }()

	router := gin.New()
	router.Use(injectTestSession())
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testaccount"})
		c.Next()
	})
	router.POST("/api/v1/accounts/:account/github/connect",
		GitHubAccountConnect(logger.New("error", "json"), pipes.New("fake-workos-key"),
			GitHubHandlerConfig{WebhookBaseURL: "https://api.astropods.ai", FrontendURL: "https://app.astropods.ai"}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/testaccount/github/connect",
		strings.NewReader(`{"redirect_to":"/new/custom"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["connected"] == true {
		t.Errorf("expected redirect_url, got connected: true — stale token was not detected")
	}
	if resp["redirect_url"] == "" || resp["redirect_url"] == nil {
		t.Errorf("expected a redirect_url for OAuth re-auth, got none: %v", resp)
	}
	if deleteHits.Load() != 1 {
		t.Errorf("expected 1 WorkOS DELETE to clear stale connection, got %d", deleteHits.Load())
	}
}

// TestGitHubAccountListRepos_StaleToken_OrgsReturns401 verifies that when WorkOS
// returns a token but GitHub rejects it with 401 on the orgs call, the handler
// returns 422 github_not_connected instead of 500.
func TestGitHubAccountListRepos_StaleToken_OrgsReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restore := installStaleGitHubTokenStub(t)
	defer restore()

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/github/repos",
		GitHubAccountListRepos(logger.New("error", "json"), pipes.New("fake-workos-key"), k8scache.NoopCache{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github/repos?q=", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "github_not_connected" {
		t.Errorf("expected github_not_connected error, got %v", resp["error"])
	}
}

// TestGitHubAccountListRepos_StaleToken_SearchReturns401 verifies that when orgs
// are cached (GetOrgs is skipped) but GitHub rejects the token with 401 during
// SearchRepos, the handler returns 422 github_not_connected instead of 500.
func TestGitHubAccountListRepos_StaleToken_SearchReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restore := installStaleGitHubTokenStub(t)
	defer restore()

	cache := mapCache{}
	orgsJSON, _ := json.Marshal([]string{"myorg"})
	cache[githubOrgsCachePrefix+"user-1"] = orgsJSON

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/api/v1/accounts/:account/github/repos",
		GitHubAccountListRepos(logger.New("error", "json"), pipes.New("fake-workos-key"), cache))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/testaccount/github/repos?q=", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "github_not_connected" {
		t.Errorf("expected github_not_connected error, got %v", resp["error"])
	}
}

// TestGitHubAccountCallback_InvalidatesOrgsCache verifies that a successful OAuth
// callback evicts any cached org list for the user so stale orgs from a prior
// GitHub account can't persist after reconnect.
func TestGitHubAccountCallback_InvalidatesOrgsCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Stub: WorkOS token endpoint returns a valid token; GitHub /user returns a login.
	mux := http.NewServeMux()
	mux.HandleFunc("/data-integrations/github/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true,
			"access_token": map[string]any{
				"access_token": "fresh-token",
				"scopes":       []string{"repo"},
			},
		})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
	})
	srv := httptest.NewServer(mux)
	old := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{server: srv}
	defer func() {
		http.DefaultTransport = old
		srv.Close()
	}()

	orgsJSON, _ := json.Marshal([]string{"old-org"})
	cache := mapCache{githubOrgsCachePrefix + "user-1": orgsJSON}

	router := gin.New()
	router.Use(injectTestSession())
	router.GET("/github/callback", GitHubAccountCallback(
		logger.New("error", "json"),
		pipes.New("fake-workos-key"),
		GitHubHandlerConfig{FrontendURL: "https://app.example.com"},
		cache,
	))

	req := httptest.NewRequest(http.MethodGet, "/github/callback?redirect_to=/new/custom", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := cache[githubOrgsCachePrefix+"user-1"]; ok {
		t.Error("expected orgs cache entry to be evicted after callback, but it still exists")
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "github_connected=true") {
		t.Errorf("expected redirect to contain github_connected=true, got %s", loc)
	}
}

// Ensure unused imports don't cause compilation errors.
var _ = bytes.NewReader
var _ = json.Marshal
