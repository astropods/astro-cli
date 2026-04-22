package agentindex

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreate_NewAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM agent_versions").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := idx.Create("acct-1", "my-agent"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_ActiveAgentReturnsErrAlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	// ON CONFLICT DO UPDATE WHERE archived_at IS NOT NULL — no rows affected when agent is active.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = idx.Create("acct-1", "my-agent")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_ArchivedAgentUnarchivesAndClearsVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	// ON CONFLICT DO UPDATE unarchives — 1 row affected.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM agent_versions").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 2)) // 2 stale versions cleared
	mock.ExpectCommit()

	if err := idx.Create("acct-1", "my-agent"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// ── SetVisibility ─────────────────────────────────────────────────────────────

// Going public permanently sets name_reserved = true via the sticky OR expression.
func TestSetVisibility_PublicSetsNameReserved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("public", sqlmock.AnyArg(), true, "acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := idx.SetVisibility("acct-1", "my-agent", "public"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Going private preserves name_reserved as-is (the OR expression evaluates to name_reserved OR false).
func TestSetVisibility_PrivatePreservesNameReserved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("private", sqlmock.AnyArg(), false, "acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := idx.SetVisibility("acct-1", "my-agent", "private"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Visibility strings other than "public" and "private" are rejected before any DB call.
func TestSetVisibility_InvalidVisibilityReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	err = idx.SetVisibility("acct-1", "my-agent", "unlisted")
	if err == nil {
		t.Fatal("expected error for invalid visibility, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// SetVisibility returns an error when the agent does not exist (0 rows affected).
func TestSetVisibility_AgentNotFoundReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("public", sqlmock.AnyArg(), true, "acct-1", "no-such-agent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := idx.SetVisibility("acct-1", "no-such-agent", "public"); err == nil {
		t.Fatal("expected error when agent not found, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// ── MarkNameReserved ──────────────────────────────────────────────────────────

// MarkNameReserved sets name_reserved = true unconditionally.
func TestMarkNameReserved_SetsFlag(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectExec("UPDATE agents SET name_reserved = true").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := idx.MarkNameReserved("acct-1", "my-agent"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// MarkNameReserved is a best-effort call — it does not return an error when the
// agent is not found (0 rows affected). The deploy handler calls it in a goroutine
// and only logs a warning on failure.
func TestMarkNameReserved_NonExistentAgentNoError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectExec("UPDATE agents SET name_reserved = true").
		WithArgs("acct-1", "no-such-agent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := idx.MarkNameReserved("acct-1", "no-such-agent"); err != nil {
		t.Fatalf("expected no error for missing agent (best-effort), got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// ── Get (name_reserved field) ─────────────────────────────────────────────────

func agentRows(accountID, name, registry, visibility string, archivedAt interface{}, nameReserved bool, ts time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at",
	}).AddRow(accountID, name, registry, visibility, archivedAt, nameReserved, nil, ts, ts)
}

func emptyVersionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at",
	})
}

// Get returns NameReserved=true when the DB row has name_reserved=true.
func TestGet_ReturnsNameReservedTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT account_id, name, registry, visibility, archived_at, name_reserved`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(agentRows("acct-1", "my-agent", "registry.example.com", "public", nil, true, now))
	mock.ExpectQuery(`SELECT build_id`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(emptyVersionRows())

	agent, err := idx.Get("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !agent.NameReserved {
		t.Error("expected NameReserved to be true")
	}
	if agent.Visibility != "public" {
		t.Errorf("expected visibility 'public', got %q", agent.Visibility)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Get returns NameReserved=false for a fresh private agent that has never been public or deployed.
func TestGet_ReturnsNameReservedFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT account_id, name, registry, visibility, archived_at, name_reserved`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(agentRows("acct-1", "my-agent", "", "private", nil, false, now))
	mock.ExpectQuery(`SELECT build_id`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(emptyVersionRows())

	agent, err := idx.Get("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.NameReserved {
		t.Error("expected NameReserved to be false for a never-public, never-deployed agent")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// Get returns NameReserved=true for an archived agent whose name is reserved
// (e.g. was previously deployed or made public before archival).
func TestGet_ArchivedButReservedNameReturnsNameReservedTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()
	archivedAt := now.Add(-24 * time.Hour)

	mock.ExpectQuery(`SELECT account_id, name, registry, visibility, archived_at, name_reserved`).
		WithArgs("acct-1", "old-agent").
		WillReturnRows(agentRows("acct-1", "old-agent", "", "private", archivedAt, true, now))
	mock.ExpectQuery(`SELECT build_id`).
		WithArgs("acct-1", "old-agent").
		WillReturnRows(emptyVersionRows())

	agent, err := idx.Get("acct-1", "old-agent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !agent.NameReserved {
		t.Error("expected NameReserved to be true for an archived agent with a reserved name")
	}
	if agent.ArchivedAt == nil {
		t.Error("expected ArchivedAt to be set")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
