package slackidentity

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// newMockStore wires a Store onto a sqlmock connection.
func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return NewStore(db), mock, db
}

const (
	upsertQuery    = "\n\t\tINSERT INTO slack_identity_mappings\n\t\t\t(team_id, slack_user_id, workos_user_id, organization_id, source,\n\t\t\t team_name, team_domain, team_icon_url, slack_username, updated_at, revoked_at)\n\t\tVALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, now(), NULL)\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE SET\n\t\t\tworkos_user_id   = EXCLUDED.workos_user_id,\n\t\t\torganization_id  = EXCLUDED.organization_id,\n\t\t\tsource           = EXCLUDED.source,\n\t\t\tteam_name        = EXCLUDED.team_name,\n\t\t\tteam_domain      = EXCLUDED.team_domain,\n\t\t\tteam_icon_url    = EXCLUDED.team_icon_url,\n\t\t\tslack_username   = EXCLUDED.slack_username,\n\t\t\tupdated_at       = now(),\n\t\t\trevoked_at       = NULL\n\t"
	lookupQuery    = "\n\t\tSELECT workos_user_id\n\t\tFROM slack_identity_mappings\n\t\tWHERE team_id = $1 AND slack_user_id = $2 AND revoked_at IS NULL\n\t\tLIMIT 1\n\t"
	listQuery      = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''), source,\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	listManyQuery  = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''), source,\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = ANY($1) AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	revokeQuery    = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t"
	revokeOneQuery = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND team_id = $2 AND revoked_at IS NULL\n\t"
)

// Upsert writes a row with all the expected fields and clears revoked_at on
// conflict — re-linking after a revoke must resurrect the mapping.
func TestUpsert_WritesRow(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertQuery).
		WithArgs("T123", "U456", "user_abc", "org_xyz", "oauth", "Acme", "acme", "https://avatars.slack-edge.com/icon.png", "alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Upsert(Mapping{
		TeamID: "T123", SlackUserID: "U456", WorkOSUserID: "user_abc",
		OrganizationID: "org_xyz", Source: "oauth",
		TeamName: "Acme", TeamDomain: "acme", TeamIconURL: "https://avatars.slack-edge.com/icon.png",
		SlackUsername: "alice",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Source defaults to "oauth" when not supplied.
func TestUpsert_DefaultsSourceToOAuth(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertQuery).
		WithArgs("T1", "U1", "user_1", "", "oauth", "", "", "", "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Upsert(Mapping{TeamID: "T1", SlackUserID: "U1", WorkOSUserID: "user_1"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Required fields are validated before hitting the DB.
func TestUpsert_RequiresKeys(t *testing.T) {
	store, _, db := newMockStore(t)
	defer db.Close()

	cases := []Mapping{
		{SlackUserID: "U1", WorkOSUserID: "u"},
		{TeamID: "T1", WorkOSUserID: "u"},
		{TeamID: "T1", SlackUserID: "U1"},
	}
	for i, m := range cases {
		if err := store.Upsert(m); err == nil {
			t.Errorf("case %d: expected error for %+v", i, m)
		}
	}
}

// Surface DB errors so callers can decide on retry/fail behavior.
func TestUpsert_PropagatesDBError(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertQuery).
		WithArgs("T1", "U1", "user_1", "", "oauth", "", "", "", "").
		WillReturnError(errors.New("boom"))

	err := store.Upsert(Mapping{TeamID: "T1", SlackUserID: "U1", WorkOSUserID: "user_1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// Lookup hits an active mapping → returns the workos user.
func TestLookup_Hit(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(lookupQuery).
		WithArgs("T1", "U1").
		WillReturnRows(sqlmock.NewRows([]string{"workos_user_id"}).AddRow("user_abc"))

	res, err := store.Lookup("T1", "U1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !res.Found || res.WorkOSUserID != "user_abc" {
		t.Errorf("got %+v", res)
	}
}

// Lookup miss is not an error — it's the common "not linked" case. The
// authorization resolver relies on this to fall through to the
// owning-account candidate without returning a 5xx.
func TestLookup_Miss_ReturnsFalseNoError(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(lookupQuery).
		WithArgs("T1", "U1").
		WillReturnError(sql.ErrNoRows)

	res, err := store.Lookup("T1", "U1")
	if err != nil {
		t.Fatalf("expected no error on miss, got %v", err)
	}
	if res.Found {
		t.Error("expected Found=false")
	}
}

// Other DB errors must propagate so a transient infra failure surfaces as
// 5xx instead of being silently treated as "not linked".
func TestLookup_PropagatesDBError(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(lookupQuery).
		WithArgs("T1", "U1").
		WillReturnError(errors.New("boom"))

	if _, err := store.Lookup("T1", "U1"); err == nil {
		t.Fatal("expected error")
	}
}

// ListByWorkOSUser returns active mappings only; tests a mix of metadata.
func TestListByWorkOSUser_ReturnsActiveMappings(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(listQuery).
		WithArgs("user_abc").
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id", "slack_user_id", "workos_user_id",
			"organization_id", "source",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}).
			AddRow("T1", "U1", "user_abc", "org_xyz", "oauth", "Acme", "acme", "https://avatars.slack-edge.com/icon.png", "alice", now, now, nil).
			AddRow("T2", "U2", "user_abc", "", "oauth", "", "", "", "", now, now, nil))

	out, err := store.ListByWorkOSUser("user_abc")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(out))
	}
	if out[0].TeamID != "T1" || out[0].OrganizationID != "org_xyz" {
		t.Errorf("first row: %+v", out[0])
	}
	if out[0].TeamName != "Acme" || out[0].TeamDomain != "acme" || out[0].SlackUsername != "alice" {
		t.Errorf("first row display fields: %+v", out[0])
	}
	if out[0].TeamIconURL != "https://avatars.slack-edge.com/icon.png" {
		t.Errorf("first row icon: %q", out[0].TeamIconURL)
	}
	if out[1].OrganizationID != "" {
		t.Errorf("second row should have empty optional org_id: %+v", out[1])
	}
	if out[1].TeamName != "" || out[1].SlackUsername != "" {
		t.Errorf("second row display fields should be empty: %+v", out[1])
	}
}

func TestListByWorkOSUser_EmptyResult(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(listQuery).
		WithArgs("user_abc").
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id", "slack_user_id", "workos_user_id",
			"organization_id", "source",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}))

	out, err := store.ListByWorkOSUser("user_abc")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %d", len(out))
	}
}

// ListByWorkOSUsers groups rows by workos_user_id so the members endpoint
// can render per-member workspace lists from a single query.
func TestListByWorkOSUsers_GroupsByUser(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(listManyQuery).
		WithArgs(pq.Array([]string{"user_a", "user_b", "user_c"})).
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id", "slack_user_id", "workos_user_id",
			"organization_id", "source",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}).
			AddRow("T1", "U1", "user_a", "", "oauth", "Acme", "acme", "", "alice", now, now, nil).
			AddRow("T2", "U2", "user_a", "", "oauth", "Foo", "foo", "", "alice", now, now, nil).
			AddRow("T3", "U3", "user_b", "", "oauth", "Bar", "bar", "", "bob", now, now, nil))

	out, err := store.ListByWorkOSUsers([]string{"user_a", "user_b", "user_c"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 keys (user_c absent), got %d", len(out))
	}
	if len(out["user_a"]) != 2 {
		t.Errorf("user_a should have 2 workspaces, got %d", len(out["user_a"]))
	}
	if len(out["user_b"]) != 1 {
		t.Errorf("user_b should have 1 workspace, got %d", len(out["user_b"]))
	}
	if _, ok := out["user_c"]; ok {
		t.Error("user_c has no rows and should be absent from the map")
	}
	if out["user_a"][0].TeamName != "Acme" {
		t.Errorf("user_a first row metadata: %+v", out["user_a"][0])
	}
}

// Empty input must not hit the DB — the caller passing an empty member list
// is the common "no members yet" case and we don't want a stray query.
func TestListByWorkOSUsers_EmptyInputSkipsDB(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	out, err := store.ListByWorkOSUsers(nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %d entries", len(out))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListByWorkOSUsers_PropagatesDBError(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(listManyQuery).
		WithArgs(pq.Array([]string{"user_a"})).
		WillReturnError(errors.New("boom"))

	if _, err := store.ListByWorkOSUsers([]string{"user_a"}); err == nil {
		t.Fatal("expected error")
	}
}

// Revoke soft-deletes every active mapping for a user and returns the count.
func TestRevoke_SoftDeletesAndReturnsCount(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(revokeQuery).
		WithArgs("user_abc").
		WillReturnResult(sqlmock.NewResult(0, 2))

	n, err := store.Revoke("user_abc")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows revoked, got %d", n)
	}
}

// Revoke on a user with no active mappings is a no-op (zero rows, no error).
func TestRevoke_NoActiveMappings(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(revokeQuery).
		WithArgs("user_abc").
		WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := store.Revoke("user_abc")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}

// RevokeOne revokes exactly one workspace and leaves the others intact.
func TestRevokeOne_ScopedToTeamID(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(revokeOneQuery).
		WithArgs("user_abc", "T1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	n, err := store.RevokeOne("user_abc", "T1")
	if err != nil {
		t.Fatalf("revoke one: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row revoked, got %d", n)
	}
}

// RevokeOne for an already-revoked or unknown team is a no-op.
func TestRevokeOne_NoMatch(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(revokeOneQuery).
		WithArgs("user_abc", "T-missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := store.RevokeOne("user_abc", "T-missing")
	if err != nil {
		t.Fatalf("revoke one: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}
