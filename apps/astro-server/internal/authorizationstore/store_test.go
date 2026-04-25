package authorizationstore

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// newMockStore builds a Store wired to a sqlmock connection.
func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return NewStore(db), mock, db
}

const (
	anyoneQuery   = "\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t"
	subjectsQuery = "\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\t  AND adapter = $2\n\t\t  AND (\n\t\t    (subject_type = 'account' AND subject_id = ANY($3))\n\t\t    OR\n\t\t    (subject_type = 'user' AND subject_id = ANY($4))\n\t\t  )\n\t\tLIMIT 1\n\t"
)

// expectAnyoneMiss queues the anyone-short-circuit query returning no rows.
func expectAnyoneMiss(mock sqlmock.Sqlmock, deploymentID, adapter string) {
	mock.ExpectQuery(anyoneQuery).
		WithArgs(deploymentID, adapter).
		WillReturnError(sql.ErrNoRows)
}

// expectAnyoneHit queues the anyone-short-circuit query returning one row.
func expectAnyoneHit(mock sqlmock.Sqlmock, deploymentID, adapter string) {
	mock.ExpectQuery(anyoneQuery).
		WithArgs(deploymentID, adapter).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
}

// isAllowedTestHelper composes the two store calls in the order the handler
// would, so tests written against the old IsAllowed shape keep their
// setup-and-assert structure.
func isAllowedTestHelper(s *Store, deploymentID string, candidates []Subject, adapter string) (bool, error) {
	if any, err := s.HasAnyoneGrant(deploymentID, adapter); err != nil {
		return false, err
	} else if any {
		return true, nil
	}
	return s.MatchesGrant(deploymentID, candidates, adapter)
}

// A6/A7 - anyone short-circuit allows immediately, no second query.
func TestIsAllowed_AnyoneGrant_AllowsAuthenticated(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneHit(mock, "dep-1", "web")

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{{Type: SubjectTypeUser, ID: "alice"}}, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true via anyone short-circuit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A7 - anyone allows even with empty candidates (anonymous traffic).
func TestIsAllowed_AnyoneGrant_AllowsAnonymous(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneHit(mock, "dep-1", "web")

	allowed, err := isAllowedTestHelper(store, "dep-1", nil, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true with empty candidates + anyone grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A1 - account grant matches a candidate account.
func TestIsAllowed_AccountGrantMatch(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "web")
	mock.ExpectQuery(subjectsQuery).
		WithArgs("dep-1", "web", pq.Array([]string{"acct-1"}), pq.Array([]string{"alice"})).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{
		{Type: SubjectTypeUser, ID: "alice"},
		{Type: SubjectTypeAccount, ID: "acct-1"},
	}, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true via account grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A3 - user grant matches the user.
func TestIsAllowed_UserGrantMatch(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "web")
	mock.ExpectQuery(subjectsQuery).
		WithArgs("dep-1", "web", pq.Array([]string(nil)), pq.Array([]string{"alice"})).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{{Type: SubjectTypeUser, ID: "alice"}}, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true via user grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A2 / A4 - no matching grant → denied.
func TestIsAllowed_NoMatch(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "web")
	mock.ExpectQuery(subjectsQuery).
		WithArgs("dep-1", "web", pq.Array([]string{"acct-1"}), pq.Array([]string{"bob"})).
		WillReturnError(sql.ErrNoRows)

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{
		{Type: SubjectTypeUser, ID: "bob"},
		{Type: SubjectTypeAccount, ID: "acct-1"},
	}, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A10 - no grants and no candidates → denied without second query.
func TestIsAllowed_EmptyCandidates_NoAnyone(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "web")

	allowed, err := isAllowedTestHelper(store, "dep-1", nil, "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false with empty candidates and no anyone grant")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A14 - slack request, no grant for slack → denied (anyone is web-only by schema).
func TestIsAllowed_SlackNoGrant(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "slack")
	mock.ExpectQuery(subjectsQuery).
		WithArgs("dep-1", "slack", pq.Array([]string{"acct-D"}), pq.Array([]string(nil))).
		WillReturnError(sql.ErrNoRows)

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{{Type: SubjectTypeAccount, ID: "acct-D"}}, "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A13 - slack with bot account grant → allowed.
func TestIsAllowed_SlackAccountGrant(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	expectAnyoneMiss(mock, "dep-1", "slack")
	mock.ExpectQuery(subjectsQuery).
		WithArgs("dep-1", "slack", pq.Array([]string{"acct-D"}), pq.Array([]string(nil))).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	allowed, err := isAllowedTestHelper(store, "dep-1", []Subject{{Type: SubjectTypeAccount, ID: "acct-D"}}, "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// IsAllowed surfaces unexpected DB errors.
func TestIsAllowed_DBError_Anyone(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(anyoneQuery).
		WithArgs("dep-1", "web").
		WillReturnError(errors.New("boom"))

	_, err := isAllowedTestHelper(store, "dep-1", []Subject{{Type: SubjectTypeUser, ID: "alice"}}, "web")
	if err == nil {
		t.Fatal("expected error to bubble up")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountIDsForUser(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT account_id FROM account_members WHERE user_id = $1\n\t").
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-1").AddRow("acct-2"))

	ids, err := store.AccountIDsForUser("alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "acct-1" || ids[1] != "acct-2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestDeploymentAccountID(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT account_id FROM deployments WHERE id = $1\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-D"))

	id, err := store.DeploymentAccountID("dep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "acct-D" {
		t.Fatalf("expected acct-D, got %q", id)
	}
}

func TestDeploymentAccountID_NotFound(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT account_id FROM deployments WHERE id = $1\n\t").
		WithArgs("dep-x").
		WillReturnError(sql.ErrNoRows)

	_, err := store.DeploymentAccountID("dep-x")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestAnyoneAdapters(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT adapter FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND subject_type = 'anyone'\n\t\tORDER BY adapter\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"adapter"}).AddRow("web"))

	adapters, err := store.AnyoneAdapters("dep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adapters) != 1 || adapters[0] != "web" {
		t.Fatalf("unexpected: %v", adapters)
	}
}

func TestAnyoneAdapters_None(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT adapter FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND subject_type = 'anyone'\n\t\tORDER BY adapter\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"adapter"}))

	adapters, err := store.AnyoneAdapters("dep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adapters) != 0 {
		t.Fatalf("expected empty, got: %v", adapters)
	}
}

func TestListGrants(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery("\n\t\tSELECT deployment_id, subject_type, subject_id, adapter\n\t\tFROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\tORDER BY subject_type, subject_id, adapter\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "subject_type", "subject_id", "adapter"}).
			AddRow("dep-1", "account", "acct-1", "web").
			AddRow("dep-1", "anyone", "", "web").
			AddRow("dep-1", "user", "alice", "web"))

	grants, err := store.ListGrants("dep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(grants) != 3 {
		t.Fatalf("expected 3 grants, got %d", len(grants))
	}
	if grants[0].SubjectType != "account" || grants[1].SubjectType != "anyone" || grants[2].SubjectType != "user" {
		t.Fatalf("unexpected ordering: %+v", grants)
	}
}

// E5/E6 - replace is atomic delete-then-insert.
func TestReplaceGrants(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("\n\t\tDELETE FROM deployment_authorization_grants WHERE deployment_id = $1\n\t").
		WithArgs("dep-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("\n\t\t\tINSERT INTO deployment_authorization_grants\n\t\t\t\t(deployment_id, subject_type, subject_id, adapter, updated_at)\n\t\t\tVALUES ($1, $2, $3, $4, now())\n\t\t").
		WithArgs("dep-1", "account", "acct-1", "web").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("\n\t\t\tINSERT INTO deployment_authorization_grants\n\t\t\t\t(deployment_id, subject_type, subject_id, adapter, updated_at)\n\t\t\tVALUES ($1, $2, $3, $4, now())\n\t\t").
		WithArgs("dep-1", "anyone", "", "web").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := store.ReplaceGrants("dep-1", []Grant{
		{SubjectType: "account", SubjectID: "acct-1", Adapter: "web"},
		{SubjectType: "anyone", SubjectID: "", Adapter: "web"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// E6 - empty grants list still runs the delete (clearing all grants).
func TestReplaceGrants_Empty(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("\n\t\tDELETE FROM deployment_authorization_grants WHERE deployment_id = $1\n\t").
		WithArgs("dep-1").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	if err := store.ReplaceGrants("dep-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Insert failure rolls back — no partial state.
func TestReplaceGrants_InsertFails(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("\n\t\tDELETE FROM deployment_authorization_grants WHERE deployment_id = $1\n\t").
		WithArgs("dep-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("\n\t\t\tINSERT INTO deployment_authorization_grants\n\t\t\t\t(deployment_id, subject_type, subject_id, adapter, updated_at)\n\t\t\tVALUES ($1, $2, $3, $4, now())\n\t\t").
		WithArgs("dep-1", "account", "acct-1", "web").
		WillReturnError(errors.New("constraint violation"))
	mock.ExpectRollback()

	err := store.ReplaceGrants("dep-1", []Grant{
		{SubjectType: "account", SubjectID: "acct-1", Adapter: "web"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
