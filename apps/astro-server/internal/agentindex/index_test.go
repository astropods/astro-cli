package agentindex

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestCreate_NewAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO agents").
		WithArgs("acct-1", sqlmock.AnyArg(), "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"uid"}).AddRow("11111111-1111-1111-1111-111111111111"))
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
	mock.ExpectQuery("INSERT INTO agents").
		WithArgs("acct-1", sqlmock.AnyArg(), "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"uid"}))
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
	mock.ExpectQuery("INSERT INTO agents").
		WithArgs("acct-1", sqlmock.AnyArg(), "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"uid"}).AddRow("11111111-1111-1111-1111-111111111111"))
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

func TestResourceID(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT uid::text").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"uid"}).AddRow("11111111-1111-1111-1111-111111111111"))

	resourceID, err := NewIndexWithDB(db).ResourceID("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("ResourceID() error = %v", err)
	}
	if resourceID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("ResourceID() = %q", resourceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestResourceIDAllowsLegacyNullUID(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT uid::text").
		WithArgs("acct-1", "legacy-agent").
		WillReturnRows(sqlmock.NewRows([]string{"uid"}).AddRow(nil))

	resourceID, err := NewIndexWithDB(db).ResourceID("acct-1", "legacy-agent")
	if err != nil {
		t.Fatalf("ResourceID() error = %v", err)
	}
	if resourceID != "" {
		t.Fatalf("ResourceID() = %q, want empty", resourceID)
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
		"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at",
	}).AddRow(accountID, name, registry, visibility, archivedAt, nameReserved, nil, nil, ts, ts)
}

func emptyVersionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at",
	})
}

// Get reads the agents row only; build payloads stay unfetched.
func TestGet_DoesNotQueryVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectQuery(`SELECT account_id, name, registry, visibility, archived_at, name_reserved`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(agentRows("acct-1", "my-agent", "", "public", nil, false, time.Now()))

	agent, err := idx.Get("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(agent.Versions) != 0 {
		t.Errorf("expected no versions loaded, got %d", len(agent.Versions))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetWithVersions_LoadsVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)
	now := time.Now()

	mock.ExpectQuery(`SELECT account_id, name, registry, visibility, archived_at, name_reserved`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(agentRows("acct-1", "my-agent", "", "public", nil, false, now))
	mock.ExpectQuery(`SELECT build_id, ecr_namespace, spec_json, readme, agent_card_json`).
		WithArgs("acct-1", "my-agent").
		WillReturnRows(emptyVersionRows().AddRow(
			"build-1", "ecr/ns", `{"meta":{"name":"a"}}`, "readme", []byte(`{}`), "", now, now,
		))

	agent, err := idx.GetWithVersions("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(agent.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(agent.Versions))
	}
	if agent.Versions[0].BuildID != "build-1" {
		t.Errorf("expected build-1, got %q", agent.Versions[0].BuildID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// ValidateLineage checks row existence without fetching build payloads.
func TestValidateLineage(t *testing.T) {
	tests := []struct {
		name    string
		rows    *sqlmock.Rows
		wantErr string
	}{
		{"exists", sqlmock.NewRows([]string{"?column?"}).AddRow(1), ""},
		{"missing", sqlmock.NewRows([]string{"?column?"}), "build not found: build-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery(`SELECT 1\s+FROM agent_versions`).
				WithArgs("acct-1", "my-agent", "build-1").
				WillReturnRows(tt.rows)

			err = NewIndexWithDB(db).ValidateLineage("acct-1", "my-agent", "build-1")
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
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

func TestExists(t *testing.T) {
	tests := []struct {
		name string
		row  bool
		want bool
	}{
		{name: "live agent", row: true, want: true},
		{name: "missing or archived agent", row: false, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM agents.*archived_at IS NULL`).
				WithArgs("acct-1", "my-agent").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(test.row))

			got, err := NewIndexWithDB(db).Exists("acct-1", "my-agent")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != test.want {
				t.Errorf("expected %v, got %v", test.want, got)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestExists_QueryFailureReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(errors.New("connection lost"))

	if _, err := NewIndexWithDB(db).Exists("acct-1", "my-agent"); err == nil {
		t.Fatal("expected an error")
	}
}

// ---------------------------------------------------------------------------
// BatchLatestBuildIDs
// ---------------------------------------------------------------------------

func TestBatchLatestBuildIDs_ReturnsOneBuildPerAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	rows := sqlmock.NewRows([]string{"account_id", "name", "build_id"}).
		AddRow("acct-1", "agent-a", "build-a-2").
		AddRow("acct-1", "agent-b", "build-b-1").
		AddRow("acct-2", "agent-c", "build-c-3")

	mock.ExpectQuery(`WITH wanted`).
		WithArgs(pq.Array([]string{"acct-1", "acct-1", "acct-2"}), pq.Array([]string{"agent-a", "agent-b", "agent-c"})).
		WillReturnRows(rows)

	got, err := idx.BatchLatestBuildIDs([]AgentVersionRef{
		{AccountID: "acct-1", Name: "agent-a"},
		{AccountID: "acct-1", Name: "agent-b"},
		{AccountID: "acct-2", Name: "agent-c"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		"acct-1/agent-a": "build-a-2",
		"acct-1/agent-b": "build-b-1",
		"acct-2/agent-c": "build-c-3",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q]=%q, want %q", k, got[k], v)
		}
	}
}

func TestBatchLatestBuildIDs_EmptyInputDoesNotQuery(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	idx := NewIndexWithDB(db)
	got, err := idx.BatchLatestBuildIDs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

func TestBatchLatestBuildIDs_FiltersZeroValueRefs(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	idx := NewIndexWithDB(db)
	got, err := idx.BatchLatestBuildIDs([]AgentVersionRef{
		{AccountID: "", Name: "x"},
		{AccountID: "acct", Name: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
	// No SQL should have been issued — confirms the function bails before
	// constructing a query with zero pairs.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB activity: %v", err)
	}
}

func TestBatchLatestBuildIDs_AbsentAgentNotInResult(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	idx := NewIndexWithDB(db)

	// Agent "agent-z" has no rows in agent_versions — server returns only
	// agent-a, and the caller treats absence as "no upgrade signal".
	mock.ExpectQuery(`WITH wanted`).
		WithArgs(pq.Array([]string{"acct-1", "acct-1"}), pq.Array([]string{"agent-a", "agent-z"})).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "build_id"}).
			AddRow("acct-1", "agent-a", "build-a-1"))

	got, err := idx.BatchLatestBuildIDs([]AgentVersionRef{
		{AccountID: "acct-1", Name: "agent-a"},
		{AccountID: "acct-1", Name: "agent-z"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["acct-1/agent-a"] != "build-a-1" {
		t.Errorf("got=%v, want build-a-1", got["acct-1/agent-a"])
	}
	if _, ok := got["acct-1/agent-z"]; ok {
		t.Error("expected absent agent to not appear in result map")
	}
}

func TestBatchLatestBuildInfoReturnsVisibility(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	idx := NewIndexWithDB(db)

	mock.ExpectQuery(`(?s)WITH wanted.*INNER JOIN agents.*INNER JOIN LATERAL.*ORDER BY v.published_at DESC.*LIMIT 1`).
		WithArgs(pq.Array([]string{"acct-1", "acct-2"}), pq.Array([]string{"private-agent", "public-agent"})).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "build_id", "visibility"}).
			AddRow("acct-1", "private-agent", "build-2", "private").
			AddRow("acct-2", "public-agent", "build-3", "public"))

	got, err := idx.BatchLatestBuildInfo(context.Background(), []AgentVersionRef{
		{AccountID: "acct-1", Name: "private-agent"},
		{AccountID: "acct-2", Name: "public-agent"},
	})
	if err != nil {
		t.Fatalf("BatchLatestBuildInfo: %v", err)
	}
	if got["acct-1/private-agent"].Visibility != "private" || got["acct-2/public-agent"].BuildID != "build-3" {
		t.Fatalf("build info = %#v", got)
	}
}

func TestBatchLatestBuildInfoHonorsCancellation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	idx := NewIndexWithDB(db)

	mock.ExpectQuery(`(?s)WITH wanted.*INNER JOIN agents.*INNER JOIN LATERAL.*ORDER BY v.published_at DESC.*LIMIT 1`).
		WithArgs(pq.Array([]string{"acct-1"}), pq.Array([]string{"agent"})).
		WillDelayFor(time.Second).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "build_id", "visibility"}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, err = idx.BatchLatestBuildInfo(ctx, []AgentVersionRef{{AccountID: "acct-1", Name: "agent"}})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("query ignored cancellation and took %s", elapsed)
	}
}

// ── Transfer ──────────────────────────────────────────────────────────────────

// Transfer rewrites agents.account_id (cascading to agent_versions and
// agent_hearts via ON UPDATE CASCADE FKs), bumps agent_versions.updated_at
// for audit, and repoints deployments.source_account_id — all in a single
// transaction. The deployments update is the lineage fix: without it,
// resolveSourceAccountName falls through to the spec-JSON fallback against
// the old account name, breaking upgrade signals.
func TestTransfer_UpdatesAgentsVersionsAndDeployments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE agents SET account_id`).
		WithArgs("target", sqlmock.AnyArg(), "source", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Audit-bump on the version rows that the FK cascade has already
	// moved to the target account. Args are (now, targetAccountID, name).
	mock.ExpectExec(`UPDATE agent_versions SET updated_at`).
		WithArgs(sqlmock.AnyArg(), "target", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`UPDATE deployments SET source_account_id`).
		WithArgs("target", "source", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	if err := idx.Transfer("source", "target", "my-agent"); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// The deployments UPDATE is keyed on (source_account_id, agent_name); rows
// for a different agent name on the same source account must not be
// touched. RowsAffected of 0 is fine — same-account deploys (where
// source_account_id == target account before the move) and unrelated
// agents fall outside the WHERE clause.
func TestTransfer_NoMatchingDeploymentsStillCommits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE agents SET account_id`).
		WithArgs("target", sqlmock.AnyArg(), "source", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE agent_versions SET updated_at`).
		WithArgs(sqlmock.AnyArg(), "target", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployments SET source_account_id`).
		WithArgs("target", "source", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := idx.Transfer("source", "target", "my-agent"); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// A failure on the deployments UPDATE must roll back the entire transfer
// (agents and agent_versions stay on the source account). Without this,
// a partial transfer leaves lineage permanently inconsistent.
func TestTransfer_DeploymentsUpdateFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE agents SET account_id`).
		WithArgs("target", sqlmock.AnyArg(), "source", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE agent_versions SET updated_at`).
		WithArgs(sqlmock.AnyArg(), "target", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployments SET source_account_id`).
		WithArgs("target", "source", "my-agent").
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	if err := idx.Transfer("source", "target", "my-agent"); err == nil {
		t.Fatal("expected error, got nil")
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
