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
		WithArgs("acct-1", "myorg", "my-agent", "user-1", "org-1", "owner/repo", "main").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Upsert(context.Background(), &Connection{
		AccountID:            "acct-1",
		AccountName:          "myorg",
		AgentName:            "my-agent",
		WorkOSUserID:         "user-1",
		WorkOSOrganizationID: "org-1",
		RepoFullName:         "owner/repo",
		Branch:               "main",
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
			"repo_full_name", "branch", "created_at", "updated_at",
		}).AddRow("conn-1", "acct-1", "myorg", "my-agent", "user-1", "org-1", "owner/repo", "main", now, now))

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

func TestRepoBase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"owner/repo", "owner/repo"},
		{"owner/repo/sub/path", "owner/repo"},
		{"owner/repo/services/a", "owner/repo"},
		{"owner", "owner"},
	}
	for _, tt := range tests {
		if got := RepoBase(tt.in); got != tt.want {
			t.Errorf("RepoBase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRepoSubPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"owner/repo", ""},
		{"owner/repo/sub/path", "sub/path"},
		{"owner/repo/services/a", "services/a"},
		{"owner", ""},
	}
	for _, tt := range tests {
		if got := RepoSubPath(tt.in); got != tt.want {
			t.Errorf("RepoSubPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStore_ListByRepoAndBranch(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	cols := []string{
		"id", "account_id", "account_name", "agent_name", "workos_user_id", "workos_org_id",
		"repo_full_name", "branch", "created_at", "updated_at",
	}
	// Both acct-1 and acct-2 connections are returned — global fan-out, no account filter.
	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("owner/repo", "main").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("c1", "acct-1", "myorg", "agent-root", "u1", "o1", "owner/repo", "main", now, now).
			AddRow("c2", "acct-2", "otherorg", "agent-svc", "u2", "o2", "owner/repo/svc", "main", now, now))

	conns, err := store.ListByRepoAndBranch(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("got %d connections, want 2", len(conns))
	}
	if conns[0].AccountID != "acct-1" {
		t.Errorf("conns[0].AccountID = %q, want acct-1", conns[0].AccountID)
	}
	if conns[1].AccountID != "acct-2" {
		t.Errorf("conns[1].AccountID = %q, want acct-2", conns[1].AccountID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestStore_ListByAccount(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now()

	mock.ExpectQuery("SELECT .+ FROM github_connections").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "repo_full_name", "created_at"}).
			AddRow("agent-a", "owner/repo-a", now).
			AddRow("agent-b", "owner/repo-b", now))

	conns, err := store.ListByAccount(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("got %d connections, want 2", len(conns))
	}
	if conns[0].AgentName != "agent-a" || conns[0].RepoFullName != "owner/repo-a" {
		t.Errorf("conns[0] = {%q, %q}, want {agent-a, owner/repo-a}", conns[0].AgentName, conns[0].RepoFullName)
	}
	if conns[1].CreatedAt.IsZero() {
		t.Errorf("conns[1].CreatedAt is zero, expected a valid timestamp")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
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
