package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
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

	mock.ExpectQuery(`SELECT .+ FROM github_connections`).
		WithArgs(repoFullName).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name",
			"workos_user_id", "workos_org_id", "repo_full_name", "branch",
			"webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow("conn-2", "acct-1", "testaccount", "test-agent",
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

// Ensure unused imports don't cause compilation errors.
var _ = bytes.NewReader
var _ = json.Marshal
