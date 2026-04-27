package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	router.POST("/webhooks/github", GitHubWebhook(log, nil, nil))

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
	router.POST("/webhooks/github", GitHubWebhook(log, nil, nil))

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
	store := githubconnection.New(db)

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("owner/unknown-repo").
		WillReturnRows(sqlmock.NewRows([]string{}))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, store, nil))

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
	store := githubconnection.New(db)

	connID := "conn-1"
	repoFullName := "owner/repo"
	webhookSecret := "correct-secret"
	now := time.Now()

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs(repoFullName).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name",
			"workos_user_id", "workos_org_id", "repo_full_name", "branch",
			"webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow(connID, "acct-1", "testaccount", "test-agent",
			"user-1", "org-1", repoFullName, "main",
			int64(12345), webhookSecret, now, now))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, store, nil))

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
	store := githubconnection.New(db)

	repoFullName := "owner/repo"
	webhookSecret := "my-secret"
	now := time.Now()

	// The push is to "feat" but the connection tracks "main".
	payloadBody := `{"ref":"refs/heads/feat","after":"abc123def456","repository":{"full_name":"owner/repo"}}`

	// GetByRepoBase: returns the connection so HMAC can be verified.
	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs(repoFullName).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name",
			"workos_user_id", "workos_org_id", "repo_full_name", "branch",
			"webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow("conn-2", "acct-1", "testaccount", "test-agent",
			"user-1", "org-1", repoFullName, "main",
			int64(12345), webhookSecret, now, now))

	// ListByRepoAndBranchForAccount: no connections for account+branch "feat" — push is ignored.
	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("acct-1", repoFullName, "feat").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name",
			"workos_user_id", "workos_org_id", "repo_full_name", "branch",
			"webhook_id", "webhook_secret", "created_at", "updated_at",
		}))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, store, nil))

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
	store := githubconnection.New(db)

	repoFullName := "owner/repo"
	webhookSecret := "my-secret"
	now := time.Now()

	// after == zeros means branch deletion.
	payloadBody := `{"ref":"refs/heads/main","after":"0000000000000000000000000000000000000000","repository":{"full_name":"owner/repo"}}`

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs(repoFullName).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name",
			"workos_user_id", "workos_org_id", "repo_full_name", "branch",
			"webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow("conn-3", "acct-1", "testaccount", "test-agent",
			"user-1", "org-1", repoFullName, "main",
			int64(12345), webhookSecret, now, now))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, store, nil))

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

func TestGitHubAccountDisconnect_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	router := gin.New()
	// No session middleware — GetSession will return false.
	router.DELETE("/api/v1/accounts/:account/github", GitHubAccountDisconnect(log, nil, nil))

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
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "webhook_id", "created_at"}).
			AddRow("my-agent", "owner/my-repo", int64(42), now))

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

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("acct-A").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "webhook_id", "created_at"}).
			AddRow("agent-a", "owner/repo", int64(42), time.Now()))

	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-A", "agent-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// count=1: a subpath connection for the same account still references webhook 42.
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("acct-A", "owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	pipesClient := pipes.New("fake-workos-key")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-A", OrganizationID: "org-A"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-A", Name: "accountA"})
		c.Next()
	})
	router.DELETE("/accounts/:account/github",
		GitHubAccountDisconnect(logger.New("error", "json"), pipesClient, store))

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

	mock.MatchExpectationsInOrder(false)

	// Get(acct-A, agent-svc) → no existing connection.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND agent_name = \$2`).
		WithArgs("acct-A", "agent-svc").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// GetByRepoForAccount(acct-A, "owner/repo/svc") → no conflict.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+WHERE account_id = \$1 AND repo_full_name = \$2`).
		WithArgs("acct-A", "owner/repo/svc").
		WillReturnRows(sqlmock.NewRows(connCols()))

	// GetByRepoBaseForAccount(acct-A, "owner/repo") → finds the root connection with webhook 99.
	mock.ExpectQuery(`SELECT .+ FROM github_connections.+account_id = \$1.+repo_full_name = \$2 OR`).
		WithArgs("acct-A", "owner/repo").
		WillReturnRows(connRow("conn-root", "acct-A", "accountA", "agent-root",
			"owner/repo", "main", "shared-secret", 99))

	// Upsert inherits webhook_id=99 and shared secret — no new webhook created.
	mock.ExpectExec("INSERT INTO github_connections").
		WithArgs(
			"acct-A", "accountA", "agent-svc", "user-1", "org-1",
			"owner/repo/svc", "main", int64(99), "shared-secret",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-A", Name: "accountA"})
		c.Next()
	})
	router.POST("/agents/:account/:name/github/link",
		GitHubLink(logger.New("error", "json"), pipes.New("fake"), store,
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

	indexMock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Get: returns service-a's connection with webhook 42 on the subpath.
	ghMock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("test-account-id", "service-a").
		WillReturnRows(connRow("conn-a", "test-account-id", "testaccount", "service-a",
			"owner/repo/service-a", "main", "shared-secret", 42))

	ghMock.ExpectExec("DELETE FROM github_connections").
		WithArgs("test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// CountByRepoBaseForAccount: 1 remaining (service-b still uses owner/repo).
	ghMock.ExpectQuery(`SELECT COUNT`).
		WithArgs("test-account-id", "owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(injectTestAccount(), injectTestSession())
	router.POST("/agents/:account/:name/archive",
		ArchiveAgent(logger.New("error", "json"), index, nil, nil, nil, store, pipes.New("fake-key")))

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

	indexMock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ghMock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs("test-account-id", "service-a").
		WillReturnRows(connRow("conn-a", "test-account-id", "testaccount", "service-a",
			"owner/repo/service-a", "main", "shared-secret", 42))

	ghMock.ExpectExec("DELETE FROM github_connections").
		WithArgs("test-account-id", "service-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// CountByRepoBaseForAccount: 0 — this was the last connection.
	ghMock.ExpectQuery(`SELECT COUNT`).
		WithArgs("test-account-id", "owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := gin.New()
	router.Use(injectTestAccount(), injectTestSession())
	router.POST("/agents/:account/:name/archive",
		ArchiveAgent(logger.New("error", "json"), index, nil, nil, nil, store, pipes.New("fake-key")))

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

// Ensure unused imports don't cause compilation errors.
var _ = bytes.NewReader
var _ = json.Marshal
