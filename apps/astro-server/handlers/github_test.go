package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
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

// --- column/row helpers ---

func connCols() []string {
	return []string{
		"id", "account_id", "account_name", "agent_name", "workos_user_id", "workos_org_id",
		"repo_full_name", "branch", "created_at", "updated_at",
	}
}

func connRow(id, accountID, orgName, agentName, repoFullName, branch string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(connCols()).AddRow(
		id, accountID, orgName, agentName, "user-1", "org-1",
		repoFullName, branch, now, now,
	)
}

func webhookCols() []string {
	return []string{"repo_base", "webhook_id", "webhook_secret", "created_at"}
}

func webhookRow(repoBase, secret string, webhookID int64) *sqlmock.Rows {
	return sqlmock.NewRows(webhookCols()).AddRow(repoBase, webhookID, secret, time.Now())
}

// setupWebhookRouter wires a push-webhook router using a single shared sqlmock DB
// for both the connection store and webhook store. Returns the mock and queue.
func setupWebhookRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, *mockGitHubBuildQueue) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)
	q := &mockGitHubBuildQueue{}
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, q))
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

	// webhookStore.Get: returns the global webhook with its secret.
	mock.ExpectQuery("SELECT .+ FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(webhookRow("owner/repo", webhookSecret, 42))

	// ListByRepoAndBranch: two connections — one per account.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "myorg", "agent-root", "u1", "o1", "owner/repo", "main", time.Now(), time.Now()).
			AddRow("c2", "acct-1", "myorg", "agent-svc", "u1", "o1", "owner/repo/svc", "main", time.Now(), time.Now()))

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
	req.Header.Set("X-Hub-Signature-256", makeHMAC(body, webhookSecret))
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

// TestGitHubWebhook_GlobalFanOut_CrossAccount verifies that a push now triggers builds
// for connections across ALL accounts, not just the account that owns the webhook.
// With a single global webhook secret, a verified push fans out globally.
func TestGitHubWebhook_GlobalFanOut_CrossAccount(t *testing.T) {
	router, mock, q := setupWebhookRouter(t)

	const webhookSecret = "global-secret"
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"README.md"})

	// Single global webhook secret for the repo.
	mock.ExpectQuery("SELECT .+ FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(webhookRow("owner/repo", webhookSecret, 42))

	// ListByRepoAndBranch returns connections from two different accounts.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("conn-A", "acct-A", "orgA", "agent-a", "u1", "o1", "owner/repo", "main", time.Now(), time.Now()).
			AddRow("conn-B", "acct-B", "orgB", "agent-b", "u2", "o2", "owner/repo", "main", time.Now(), time.Now()))

	// Build for each connection.
	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-A"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("INSERT INTO github_builds").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-B"))
	mock.ExpectExec("UPDATE github_builds").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", makeHMAC(body, webhookSecret))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(q.enqueued) != 2 {
		t.Errorf("expected 2 enqueued jobs (one per account), got %d", len(q.enqueued))
	}
	// Verify both accounts' connections were enqueued — conn-A belongs to acct-A, conn-B to acct-B.
	connIDs := map[string]bool{}
	for _, e := range q.enqueued {
		connIDs[e.ConnectionID] = true
	}
	if !connIDs["conn-A"] || !connIDs["conn-B"] {
		t.Errorf("expected builds for conn-A (acct-A) and conn-B (acct-B); got %v", connIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// TestGitHubWebhook_SubPathConnectionBuildsRegardlessOfChangedFiles verifies that
// every connection for the pushed repo+branch receives a build, including subpath
// connections, regardless of which files were changed.
func TestGitHubWebhook_SubPathConnectionBuildsRegardlessOfChangedFiles(t *testing.T) {
	router, mock, q := setupWebhookRouter(t)

	const webhookSecret = "topsecret"
	// Push only touches README.md — nothing under svc/.
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"README.md"})

	mock.ExpectQuery("SELECT .+ FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(webhookRow("owner/repo", webhookSecret, 42))

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "myorg", "agent-root", "u1", "o1", "owner/repo", "main", time.Now(), time.Now()).
			AddRow("c2", "acct-1", "myorg", "agent-svc", "u1", "o1", "owner/repo/svc", "main", time.Now(), time.Now()))

	// Both connections must build — root and svc subpath.
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
	req.Header.Set("X-Hub-Signature-256", makeHMAC(body, webhookSecret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(q.enqueued) != 2 {
		t.Errorf("expected 2 builds (root + svc subpath), got %d", len(q.enqueued))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// --- Unit tests for subpath validation helpers ---

func TestValidateRepoFullName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid owner/repo", "owner/repo", false},
		{"valid with single subpath segment", "owner/repo/svc", false},
		{"valid with multi-segment subpath", "owner/repo/services/my-agent", false},
		{"missing repo segment", "owner", true},
		{"empty string", "", true},
		{"dot-dot segment", "owner/repo/..", true},
		{"dot segment", "owner/repo/.", true},
		{"empty segment from double slash", "owner//repo", true},
		{"subpath with dot-dot", "owner/repo/../etc", true},
		{"subpath with empty component", "owner/repo//svc", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRepoFullName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateRepoFullName(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// --- TestGitHubWebhook_PartialFanOutFailure ---

var errPartialFailureSim = &stubError{"simulated transient db error"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

// TestGitHubWebhook_PartialFanOutFailureIsNotAccepted verifies that when fan-out
// enqueues at least one build but another connection's CreateBuild fails,
// the handler does not return 202 so GitHub can surface the partial failure.
func TestGitHubWebhook_PartialFanOutFailureIsNotAccepted(t *testing.T) {
	router, mock, _ := setupWebhookRouter(t)

	const webhookSecret = "secret"
	body := pushPayload("owner/repo", "main", "deadbeef", []string{"svc/main.go"})

	mock.ExpectQuery("SELECT .+ FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(webhookRow("owner/repo", webhookSecret, 42))

	// Two connections both match.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(connCols()).
			AddRow("c1", "acct-1", "org1", "agent-root", "u1", "o1", "owner/repo", "main", time.Now(), time.Now()).
			AddRow("c2", "acct-1", "org1", "agent-svc", "u1", "o1", "owner/repo/svc", "main", time.Now(), time.Now()))

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
	req.Header.Set("X-Hub-Signature-256", makeHMAC(body, webhookSecret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusAccepted {
		t.Errorf("expected non-202 on partial fan-out failure; got %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet SQL expectations: %v", err)
	}
}

// --- GitHubDisconnect tests ---

func setupDisconnectTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{UserID: "user-1", OrganizationID: "org-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "myorg"})
		c.Next()
	})
	router.DELETE("/agents/:account/:name/github", GitHubDisconnect(log, nil, connStore, whStore))
	return router, mock
}

// TestGitHubDisconnect_KeepsWebhookWhenShared verifies that disconnecting one connection
// does not delete the global webhook when other connections still reference that base repo.
func TestGitHubDisconnect_KeepsWebhookWhenShared(t *testing.T) {
	router, mock := setupDisconnectTest(t)

	// Get connection.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(connRow("conn-1", "acct-1", "myorg", "my-agent", "owner/repo/svc", "main"))

	// Delete connection.
	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections: another connection still references owner/repo — no rows returned.
	mock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}))

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

// TestGitHubWebhook_WebhookStoreLookupError verifies that a non-ErrNoRows error from
// webhookStore.Get causes the handler to return 500 rather than silently dropping the push.
func TestGitHubWebhook_WebhookStoreLookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	connStore := githubconnection.New(db)
	whStore := githubwebhook.New(db)

	mock.ExpectQuery(`SELECT .+ FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnError(fmt.Errorf("db error"))

	router := gin.New()
	router.POST("/webhooks/github", GitHubWebhook(log, connStore, whStore, nil))

	payload := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"owner/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestGitHubDisconnect_DeletesWebhookWhenLast verifies that disconnecting the last
// connection for a base repo triggers deletion of the global webhook row.
// (Actual GitHub API call is best-effort and skipped here because pipesClient is nil.)
func TestGitHubDisconnect_DeletesWebhookWhenLast(t *testing.T) {
	router, mock := setupDisconnectTest(t)

	// Get connection.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(connRow("conn-1", "acct-1", "myorg", "my-agent", "owner/repo", "main"))

	// Delete connection.
	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// DeleteIfNoConnections: no remaining connections — returns the webhook_id.
	mock.ExpectQuery("DELETE FROM github_webhooks").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}).AddRow(int64(7)))

	// pipesClient is nil so the actual GitHub API call is skipped — still 204.
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
