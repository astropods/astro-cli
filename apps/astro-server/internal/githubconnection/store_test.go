package githubconnection

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newTestStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db), mock
}

func TestStore_Upsert(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO github_connections").
		WithArgs("acct-1", "myorg", "my-agent", "user-1", "org-1", "owner/repo", "main", int64(42), "secret123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Upsert(context.Background(), &Connection{
		AccountID:            "acct-1",
		AccountName:          "myorg",
		AgentName:            "my-agent",
		WorkOSUserID:         "user-1",
		WorkOSOrganizationID: "org-1",
		RepoFullName:         "owner/repo",
		Branch:               "main",
		WebhookID:            42,
		WebhookSecret:        "secret123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_Get_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name", "workos_user_id", "workos_org_id",
			"repo_full_name", "branch", "webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow("conn-1", "acct-1", "myorg", "my-agent", "user-1", "org-1", "owner/repo", "main", int64(42), "s3cr3t", now, now))

	conn, err := store.Get(context.Background(), "acct-1", "my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "conn-1" {
		t.Errorf("ID = %q, want %q", conn.ID, "conn-1")
	}
	if conn.RepoFullName != "owner/repo" {
		t.Errorf("RepoFullName = %q, want %q", conn.RepoFullName, "owner/repo")
	}
	if conn.WebhookID != 42 {
		t.Errorf("WebhookID = %d, want 42", conn.WebhookID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1", "missing-agent").
		WillReturnRows(sqlmock.NewRows([]string{})) // empty result set

	_, err := store.Get(context.Background(), "acct-1", "missing-agent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestStore_GetByRepo_Success(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "account_name", "agent_name", "workos_user_id", "workos_org_id",
			"repo_full_name", "branch", "webhook_id", "webhook_secret", "created_at", "updated_at",
		}).AddRow("conn-2", "acct-1", "myorg", "my-agent", "user-1", "org-1", "owner/repo", "main", int64(7), "tok", now, now))

	conn, err := store.GetByRepo(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.ID != "conn-2" {
		t.Errorf("ID = %q, want %q", conn.ID, "conn-2")
	}
}

func TestStore_Delete(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("DELETE FROM github_connections").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Delete(context.Background(), "acct-1", "my-agent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_CreateBuild(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery("INSERT INTO github_builds").
		WithArgs("conn-1", "acct-1", "my-agent", "build-abc", "deadbeef", "main", "pending", "fetching-spec", "feat: add feature", "Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("build-row-1"))

	id, err := store.CreateBuild(context.Background(), &Build{
		ConnectionID:  "conn-1",
		AccountID:     "acct-1",
		AgentName:     "my-agent",
		BuildID:       "build-abc",
		CommitSHA:     "deadbeef",
		Branch:        "main",
		Status:        "pending",
		Step:          "fetching-spec",
		CommitMessage: "feat: add feature",
		CommitAuthor:  "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "build-row-1" {
		t.Errorf("id = %q, want %q", id, "build-row-1")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_UpdateBuildStatus_NoError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE github_builds").
		WithArgs("registered", "build-row-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateBuildStatus(context.Background(), "build-row-1", "registered", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_UpdateBuildStatus_WithError(t *testing.T) {
	store, mock := newTestStore(t)

	// When buildErr is non-empty, a different query branch is used.
	mock.ExpectExec("UPDATE github_builds").
		WithArgs("failed", "build failed: missing Dockerfile", "build-row-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateBuildStatus(context.Background(), "build-row-1", "failed", "build failed: missing Dockerfile"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_UpdateBuildStep(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("UPDATE github_builds SET step").
		WithArgs("building", "build-row-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateBuildStep(context.Background(), "build-row-1", "building"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_ListBuilds(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectQuery("SELECT .+ FROM github_builds").
		WithArgs("acct-1", "my-agent", 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "connection_id", "account_id", "agent_name", "build_id", "commit_sha",
			"branch", "status", "step", "commit_message", "commit_author", "error", "enqueued_at", "completed_at",
		}).
			AddRow("r1", "conn-1", "acct-1", "my-agent", "b1", "sha1", "main", "registered", "registering", "feat A", "Bob", "", now, nil).
			AddRow("r2", "conn-1", "acct-1", "my-agent", "b2", "sha2", "main", "failed", "building", "feat B", "Alice", "build error", now, &now))

	builds, err := store.ListBuilds(context.Background(), "acct-1", "my-agent", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(builds) != 2 {
		t.Fatalf("got %d builds, want 2", len(builds))
	}
	if builds[0].Status != "registered" {
		t.Errorf("builds[0].Status = %q, want %q", builds[0].Status, "registered")
	}
	if builds[1].Error != "build error" {
		t.Errorf("builds[1].Error = %q, want %q", builds[1].Error, "build error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_ListBuilds_DefaultLimit(t *testing.T) {
	store, mock := newTestStore(t)

	// limit <= 0 should default to 10
	mock.ExpectQuery("SELECT .+ FROM github_builds").
		WithArgs("acct-1", "my-agent", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "connection_id", "account_id", "agent_name", "build_id", "commit_sha",
			"branch", "status", "step", "commit_message", "commit_author", "error", "enqueued_at", "completed_at",
		}))

	_, err := store.ListBuilds(context.Background(), "acct-1", "my-agent", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
