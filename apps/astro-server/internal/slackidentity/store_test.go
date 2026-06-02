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
	upsertQuery         = "\n\t\tINSERT INTO slack_identity_mappings\n\t\t\t(team_id, slack_user_id, workos_user_id, organization_id, source,\n\t\t\t team_name, team_domain, team_icon_url, slack_username, updated_at, revoked_at)\n\t\tVALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, now(), NULL)\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE SET\n\t\t\tworkos_user_id   = EXCLUDED.workos_user_id,\n\t\t\torganization_id  = EXCLUDED.organization_id,\n\t\t\tsource           = EXCLUDED.source,\n\t\t\tteam_name        = EXCLUDED.team_name,\n\t\t\tteam_domain      = EXCLUDED.team_domain,\n\t\t\tteam_icon_url    = EXCLUDED.team_icon_url,\n\t\t\tslack_username   = EXCLUDED.slack_username,\n\t\t\tupdated_at       = now(),\n\t\t\trevoked_at       = NULL\n\t"
	upsertObservedQuery = "\n\t\tINSERT INTO slack_identity_mappings\n\t\t\t(team_id, slack_user_id, workos_user_id, source)\n\t\tVALUES ($1, $2, NULL, 'observed')\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE\n\t\tSET revoked_at = NULL,\n\t\t    updated_at = now()\n\t\tWHERE slack_identity_mappings.revoked_at IS NOT NULL\n\t\t  AND slack_identity_mappings.source     = 'observed'\n\t"
	lookupQuery         = "\n\t\tSELECT workos_user_id\n\t\tFROM slack_identity_mappings\n\t\tWHERE team_id = $1 AND slack_user_id = $2\n\t\t  AND workos_user_id IS NOT NULL\n\t\t  AND revoked_at IS NULL\n\t\tLIMIT 1\n\t"
	listQuery           = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''), source,\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	listManyQuery       = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''), source,\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = ANY($1) AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	revokeQuery         = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t"
	revokeOneQuery      = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND team_id = $2 AND revoked_at IS NULL\n\t"
	directoryEntries    = "\n\t\tSELECT DISTINCT ON (slack_user_id) slack_user_id, team_id, COALESCE(workos_user_id, '')\n\t\tFROM slack_identity_mappings\n\t\tWHERE slack_user_id = ANY($1) AND revoked_at IS NULL\n\t\tORDER BY slack_user_id, created_at DESC\n\t"
	listAccountTeams    = "\n\t\tSELECT DISTINCT am.account_id, sim.team_id\n\t\tFROM slack_identity_mappings sim\n\t\tJOIN account_members am ON sim.workos_user_id = am.user_id\n\t\tWHERE sim.workos_user_id IS NOT NULL\n\t\t  AND sim.revoked_at IS NULL\n\t\tORDER BY am.account_id, sim.team_id\n\t"
	checkMarker         = "SELECT EXISTS(SELECT 1 FROM slack_directory_backfill_marker)"
	writeMarker         = "\n\t\tINSERT INTO slack_directory_backfill_marker (id, completed_at)\n\t\tVALUES (1, now())\n\t\tON CONFLICT (id) DO NOTHING\n\t"
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

// ── UpsertObserved ──────────────────────────────────────────────────────────

// First call for an (team, user) pair writes the observed row.
func TestUpsertObserved_FirstCallWrites(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("upsert observed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Subsequent calls for the same pair within one process are deduped — the
// in-memory cache short-circuits before touching the DB. Critical for
// keeping write amplification bounded on a chatty Slack workspace.
func TestUpsertObserved_PerProcessDedupe(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	// Only one DB call expected for two upserts.
	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("second upsert (should be a no-op): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expected exactly 1 DB call: %v", err)
	}
}

// A revoked observed row must be revived in-place when the user
// re-appears via authorize — covers the ops-rollback case where someone
// ran the documented `UPDATE … SET revoked_at = now() WHERE source =
// 'observed'` and then traffic resumes. sqlmock just verifies the query
// shape; Postgres semantics enforce the "observed + revoked_at IS NOT
// NULL" guard at execution time via the WHERE on the DO UPDATE branch.
func TestUpsertObserved_RevivesRevokedObservedRow(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row updated (revived)

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("upsert observed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// SECURITY: a revoked oauth row must NOT be revived by passive
// observation — revocation is a deliberate user action ("I disconnected
// my Slack from Astro") and reviving it silently on the next message
// would re-attribute their messages back to the old WorkOS account
// behind their back. The WHERE clause guards on source='observed' so
// the DO UPDATE branch doesn't match oauth rows; Postgres returns 0
// rows affected and the original revoked oauth row stays put.
//
// This pins the SQL we issue. The actual revival semantics are
// enforced by the WHERE clause at execution time — sqlmock doesn't
// emulate WHERE filtering, but the matching constant ensures we'd
// catch any future code change that broadens the WHERE to revive
// oauth rows.
func TestUpsertObserved_DoesNotReviveRevokedOAuthRow(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	// Postgres would return RowsAffected=0 because the WHERE clause on
	// the DO UPDATE branch (source='observed') fails to match an
	// oauth row. The INSERT itself conflicts on the unique
	// (team_id, slack_user_id) key, so no new row is created either.
	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(0, 0)) // no rows affected — revocation respected

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("upsert observed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Memory cap: the in-process dedupe map resets after
// observedSeenResetInterval, so a long-running pod doesn't accumulate
// an unbounded set of (team, user) keys. A user we already deduped
// re-pays the DB UPSERT on the next call past the reset boundary;
// safe because the SQL is idempotent.
func TestUpsertObserved_PeriodicResetClearsDedupe(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()
	now := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return now }
	store.observedLastReset = now

	// Two DB calls expected: one before the reset, one after — the
	// dedupe map is wiped in between so the second call doesn't hit
	// the in-memory cache.
	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	now = now.Add(25 * time.Hour) // past the 24h reset interval
	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Fatalf("upsert after reset: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expected 2 DB calls (dedupe wiped between them): %v", err)
	}
}

// Empty inputs are tolerated silently — the authorize handler tolerates
// them too (anonymous traffic).
func TestUpsertObserved_EmptyInputsAreNoOp(t *testing.T) {
	store, _, db := newMockStore(t)
	defer db.Close()

	if err := store.UpsertObserved(t.Context(), "", "U07ABC"); err != nil {
		t.Errorf("empty team: %v", err)
	}
	if err := store.UpsertObserved(t.Context(), "T07XYZ", ""); err != nil {
		t.Errorf("empty user: %v", err)
	}
	// No mock expectations → no DB calls made.
}

// DB error rolls back the in-memory dedupe so the next call retries.
// Otherwise a transient DB hiccup would suppress this user forever.
func TestUpsertObserved_DBErrorRollsBackDedupe(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnError(errors.New("temporary db hiccup"))
	mock.ExpectExec(upsertObservedQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err == nil {
		t.Error("expected error on first call")
	}
	if err := store.UpsertObserved(t.Context(), "T07XYZ", "U07ABCDEF"); err != nil {
		t.Errorf("retry after error must hit DB again, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expected 2 DB calls: %v", err)
	}
}

// ── Lookup excludes observed-only rows ──────────────────────────────────────

// Lookup is for identity resolution (linked users only). An observed-only
// row has no WorkOS user to resolve to and must NOT count as a Found hit —
// the handler would otherwise flow through the "linked" branch with an
// empty user_id, breaking attribution.
func TestLookup_ExcludesObservedOnlyRows(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	// Lookup query now filters on workos_user_id IS NOT NULL — an
	// observed-only row (workos_user_id IS NULL) doesn't match, so the
	// query returns no rows.
	mock.ExpectQuery(lookupQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnError(sql.ErrNoRows)

	res, err := store.Lookup("T07XYZ", "U07ABCDEF")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if res.Found {
		t.Error("observed-only row must not surface as Found")
	}
}

// ── DirectoryEntriesForSlackUsers ────────────────────────────────────────────

// Linked Slack users return workos_user_id; observed-only return team only.
// Insights uses both: workos_user_id triggers the merge into the WorkOS
// row; team_id alone drives the deep link for unmapped users.
func TestDirectoryEntriesForSlackUsers_MixedRows(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectQuery(directoryEntries).
		WithArgs(pq.Array([]string{"U07LINKED", "U07OBSERVED"})).
		WillReturnRows(sqlmock.NewRows([]string{"slack_user_id", "team_id", "workos_user_id"}).
			AddRow("U07LINKED", "T07XYZ", "user_alice").
			AddRow("U07OBSERVED", "T07XYZ", ""))

	out, err := store.DirectoryEntriesForSlackUsers([]string{"U07LINKED", "U07OBSERVED"})
	if err != nil {
		t.Fatalf("directory entries: %v", err)
	}
	if got := out["U07LINKED"]; got.TeamID != "T07XYZ" || got.WorkOSUserID != "user_alice" {
		t.Errorf("linked entry: got %+v", got)
	}
	if got := out["U07OBSERVED"]; got.TeamID != "T07XYZ" || got.WorkOSUserID != "" {
		t.Errorf("observed entry should leave WorkOSUserID empty: got %+v", got)
	}
}

// Empty input returns an empty map without a DB round-trip.
func TestDirectoryEntriesForSlackUsers_EmptyInput(t *testing.T) {
	store, _, db := newMockStore(t)
	defer db.Close()

	out, err := store.DirectoryEntriesForSlackUsers(nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %d entries", len(out))
	}
}

// ── One-shot backfill marker ───────────────────────────────────────────────

// Marker absent → IsDirectoryBackfillComplete returns false; the worker
// runs and does its work.
func TestIsDirectoryBackfillComplete_Absent(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()
	mock.ExpectQuery(checkMarker).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	done, err := store.IsDirectoryBackfillComplete(t.Context())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if done {
		t.Error("expected done=false with no marker row")
	}
}

// Marker present → IsDirectoryBackfillComplete returns true; the worker
// must exit without touching anything (the "never runs again" guarantee).
func TestIsDirectoryBackfillComplete_Present(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()
	mock.ExpectQuery(checkMarker).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	done, err := store.IsDirectoryBackfillComplete(t.Context())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !done {
		t.Error("expected done=true with marker row")
	}
}

// Writing the marker is idempotent — concurrent writes from two pods
// during a rolling deploy must not error. ON CONFLICT DO NOTHING covers
// the race; the PRIMARY KEY + singleton CHECK make the table physically
// single-row.
func TestMarkDirectoryBackfillComplete_IdempotentWrite(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()
	mock.ExpectExec(writeMarker).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.MarkDirectoryBackfillComplete(t.Context()); err != nil {
		t.Fatalf("mark complete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
