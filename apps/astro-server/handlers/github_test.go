package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/gin-gonic/gin"
)

// --- mockGitHubBuildQueue ---

type mockGitHubBuildQueue struct {
	enqueued  []riverqueue.GitHubBuildArgs
	cancelled []string
}

func (m *mockGitHubBuildQueue) EnqueueGitHubBuild(_ context.Context, args riverqueue.GitHubBuildArgs) error {
	m.enqueued = append(m.enqueued, args)
	return nil
}

func (m *mockGitHubBuildQueue) CancelGitHubBuildsForConnection(_ context.Context, connectionID string) {
	m.cancelled = append(m.cancelled, connectionID)
}

// --- helper: compute HMAC-SHA256 signature ---

func githubSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// --- webhook handler test helpers ---

func connCols() []string {
	return []string{
		"id", "account_id", "account_name", "agent_name", "workos_user_id", "workos_org_id",
		"repo_full_name", "branch", "webhook_id", "webhook_secret", "created_at", "updated_at",
	}
}

func connRow(id, accountID, orgName, agentName, repoFullName, branch, webhookSecret string, webhookID int64) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(connCols()).AddRow(
		id, accountID, orgName, agentName, "user-1", "org-1",
		repoFullName, branch, webhookID, webhookSecret, now, now,
	)
}

func setupWebhookRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, *mockGitHubBuildQueue) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := githubconnection.New(db)
	q := &mockGitHubBuildQueue{}
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, store, q))
	return router, mock, q
}

// pushPayload builds a minimal GitHub push event JSON body.
func pushPayload(repoFullName, branch, commitSHA string, changedFiles []string) []byte {
	commits := []map[string]interface{}{
		{
			"added":    changedFiles,
			"removed":  []string{},
			"modified": []string{},
		},
	}
	payload := map[string]interface{}{
		"ref":   "refs/heads/" + branch,
		"after": commitSHA,
		"repository": map[string]string{
			"full_name": repoFullName,
		},
		"head_commit": map[string]interface{}{
			"message": "feat: add feature",
			"author":  map[string]string{"name": "Alice"},
		},
		"commits": commits,
	}
	b, _ := json.Marshal(payload)
	return b
}

// --- TestGitHubWebhook_FanOut ---

func TestGitHubWebhook_FanOut(t *testing.T) {
	router, mock, q := setupWebhookRouter(t)

	const webhookSecret = "topsecret"
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"svc/main.go", "README.md"})

	// GetByRepoBase: returns the base connection with webhook secret.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo").
		WillReturnRows(connRow("conn-base", "acct-1", "myorg", "agent-root", "owner/repo", "main", webhookSecret, 42))

	// ListByRepoAndBranch: returns two connections.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "myorg", "agent-root", "u1", "o1", "owner/repo", "main", int64(42), webhookSecret, time.Now(), time.Now()).
			AddRow("c2", "acct-1", "myorg", "agent-svc", "u1", "o1", "owner/repo/svc", "main", int64(42), webhookSecret, time.Now(), time.Now()))

	// CreateBuild called twice (one per connection).
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-1"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-2"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", githubSig(webhookSecret, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(q.enqueued) != 2 {
		t.Errorf("expected 2 enqueued jobs, got %d", len(q.enqueued))
	}
}

// --- TestGitHubWebhook_PathFiltering ---

func TestGitHubWebhook_PathFiltering(t *testing.T) {
	router, mock, q := setupWebhookRouter(t)

	const webhookSecret = "topsecret"
	// Only README.md changed — does not match "svc/" prefix.
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"README.md"})

	// GetByRepoBase.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo").
		WillReturnRows(connRow("conn-base", "acct-1", "myorg", "agent-root", "owner/repo", "main", webhookSecret, 42))

	// ListByRepoAndBranch: two connections (root + svc subpath).
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "myorg", "agent-root", "u1", "o1", "owner/repo", "main", int64(42), webhookSecret, time.Now(), time.Now()).
			AddRow("c2", "acct-1", "myorg", "agent-svc", "u1", "o1", "owner/repo/svc", "main", int64(42), webhookSecret, time.Now(), time.Now()))

	// Only the root connection (empty subpath) triggers a build.
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-1"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", githubSig(webhookSecret, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(q.enqueued) != 1 {
		t.Errorf("expected 1 enqueued job (path filtering skipped svc), got %d", len(q.enqueued))
	}
}

// --- TestGitHubDisconnect tests ---

func setupDisconnectTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := githubconnection.New(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "myorg"})
		c.Next()
	})
	router.DELETE("/agents/:account/:name/github", GitHubDisconnect(log, nil, store))
	return router, mock
}

func TestGitHubDisconnect_KeepsWebhookWhenShared(t *testing.T) {
	router, mock := setupDisconnectTest(t)
	now := time.Now()

	// Get connection.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(connCols()).AddRow(
			"conn-1", "acct-1", "myorg", "my-agent", "u1", "o1",
			"owner/repo/svc", "main", int64(7), "secret", now, now,
		))

	// Delete connection.
	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// CountByRepoBase returns 1 (another connection still uses the webhook).
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodDelete, "/agents/myorg/my-agent/github", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// pushPayloadWithCommits builds a GitHub push event with an explicit commits slice.
func pushPayloadWithCommits(repoFullName, branch, commitSHA string, commits []map[string]any) []byte {
	payload := map[string]any{
		"ref":        "refs/heads/" + branch,
		"after":      commitSHA,
		"repository": map[string]string{"full_name": repoFullName},
		"head_commit": map[string]any{
			"message": "bulk",
			"author":  map[string]string{"name": "Alice"},
		},
		"commits": commits,
	}
	b, _ := json.Marshal(payload)
	return b
}

// truncatedCommits returns exactly n commits, none of which touch any subpath.
// Mirrors GitHub's behavior of truncating the commits array at 20.
func truncatedCommits(n int) []map[string]any {
	out := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, map[string]any{
			"added":    []string{"docs/unrelated-" + strconv.Itoa(i) + ".md"},
			"removed":  []string{},
			"modified": []string{},
		})
	}
	return out
}

var errPartialFailureSim = &stubError{"simulated transient db error"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

// TestGitHubWebhook_TruncatedCommitsEnqueueSubPath verifies that a push with exactly
// 20 commits (GitHub's truncation cap) still enqueues a build for subpath connections.
// When the commits list is saturated, the handler cannot trust it for path filtering.
func TestGitHubWebhook_TruncatedCommitsEnqueueSubPath(t *testing.T) {
	router, mock, q := setupWebhookRouter(t)

	const webhookSecret = "shared-secret"
	body := pushPayloadWithCommits("owner/repo", "main", "abc1234", truncatedCommits(20))

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo").
		WillReturnRows(connRow("conn-base", "acct-1", "org1", "agent-svc",
			"owner/repo/svc", "main", webhookSecret, 42))

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "org1", "agent-svc", "u1", "o1",
				"owner/repo/svc", "main", int64(42), webhookSecret, time.Now(), time.Now()))

	// A fixed handler should enqueue a build for the subpath connection
	// despite none of the 20 (truncated) commits touching svc/.
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-1"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", githubSig(webhookSecret, body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if len(q.enqueued) == 0 {
		t.Errorf("expected build enqueued for subpath connection on 20-commit (truncated) push, got 0")
	}
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 when a build is enqueued, got %d: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("CreateBuild / UpdateBuildStatus expected but not reached: %v", err)
	}
}

// TestGitHubWebhook_PartialFanOutFailureIsNotAccepted verifies that when fan-out
// enqueues at least one build but another connection's CreateBuild fails,
// the handler does not return 202 so GitHub can surface the partial failure.
func TestGitHubWebhook_PartialFanOutFailureIsNotAccepted(t *testing.T) {
	router, mock, _ := setupWebhookRouter(t)

	const webhookSecret = "secret"
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"svc/main.go"})

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo").
		WillReturnRows(connRow("conn-base", "acct-1", "org1", "agent-root",
			"owner/repo", "main", webhookSecret, 42))

	// Two connections both match — root + subpath.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "org1", "agent-root", "u1", "o1",
				"owner/repo", "main", int64(42), webhookSecret, time.Now(), time.Now()).
			AddRow("c2", "acct-1", "org1", "agent-svc", "u1", "o1",
				"owner/repo/svc", "main", int64(42), webhookSecret, time.Now(), time.Now()))

	// First CreateBuild succeeds.
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-1"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Second CreateBuild fails — simulates a transient DB error on the subpath connection.
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnError(errPartialFailureSim)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", githubSig(webhookSecret, body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusAccepted {
		t.Errorf("expected non-202 on partial fan-out failure; got %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

func TestGitHubDisconnect_DeletesWebhookWhenLast(t *testing.T) {
	router, mock := setupDisconnectTest(t)
	now := time.Now()

	// Get connection.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(connCols()).AddRow(
			"conn-1", "acct-1", "myorg", "my-agent", "u1", "o1",
			"owner/repo", "main", int64(7), "secret", now, now,
		))

	// Delete connection.
	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// CountByRepoBase returns 0 (no more connections for this base repo).
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Pipes call will fail (nil pipesClient) → webhook deletion is best-effort, still 204.
	req := httptest.NewRequest(http.MethodDelete, "/agents/myorg/my-agent/github", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
