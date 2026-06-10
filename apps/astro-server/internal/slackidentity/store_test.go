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
	upsertQuery    = "\n\t\tINSERT INTO slack_identity_mappings\n\t\t\t(team_id, slack_user_id, workos_user_id, organization_id,\n\t\t\t team_name, team_domain, team_icon_url, slack_username, updated_at, revoked_at)\n\t\tVALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, now(), NULL)\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE SET\n\t\t\tworkos_user_id   = EXCLUDED.workos_user_id,\n\t\t\torganization_id  = EXCLUDED.organization_id,\n\t\t\tteam_name        = EXCLUDED.team_name,\n\t\t\tteam_domain      = EXCLUDED.team_domain,\n\t\t\tteam_icon_url    = EXCLUDED.team_icon_url,\n\t\t\tslack_username   = EXCLUDED.slack_username,\n\t\t\tupdated_at       = now(),\n\t\t\trevoked_at       = NULL\n\t"
	lookupQuery    = "\n\t\tSELECT workos_user_id\n\t\tFROM slack_identity_mappings\n\t\tWHERE team_id = $1 AND slack_user_id = $2\n\t\t  AND workos_user_id IS NOT NULL\n\t\t  AND revoked_at IS NULL\n\t\tLIMIT 1\n\t"
	listQuery      = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''),\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	listManyQuery  = "\n\t\tSELECT team_id, slack_user_id, workos_user_id,\n\t\t       COALESCE(organization_id, ''),\n\t\t       team_name, team_domain, team_icon_url, slack_username,\n\t\t       created_at, updated_at, revoked_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE workos_user_id = ANY($1) AND revoked_at IS NULL\n\t\tORDER BY created_at DESC\n\t"
	revokeQuery    = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND revoked_at IS NULL\n\t"
	revokeOneQuery = "\n\t\tUPDATE slack_identity_mappings\n\t\tSET revoked_at = now(), updated_at = now()\n\t\tWHERE workos_user_id = $1 AND team_id = $2 AND revoked_at IS NULL\n\t"
	// Sole observed-write target after PR 2 cutover.
	upsertObservedUserQuery     = "\n\t\tINSERT INTO slack_observed_users (team_id, slack_user_id)\n\t\tVALUES ($1, $2)\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE\n\t\tSET last_seen_at = now()\n\t"
	upsertObservedProfilesQuery = "\n\t\tWITH input AS (\n\t\t\tSELECT *\n\t\t\tFROM unnest(\n\t\t\t\t$1::text[],\n\t\t\t\t$2::text[],\n\t\t\t\t$3::text[],\n\t\t\t\t$4::text[],\n\t\t\t\t$5::text[],\n\t\t\t\t$6::boolean[],\n\t\t\t\t$7::boolean[]\n\t\t\t) AS t(team_id, slack_user_id, slack_display_name, slack_username, slack_avatar_url, slack_is_bot, slack_deleted)\n\t\t)\n\t\tINSERT INTO slack_observed_users\n\t\t\t(team_id, slack_user_id, slack_display_name, slack_username,\n\t\t\t slack_avatar_url, slack_is_bot, slack_deleted, profile_updated_at)\n\t\tSELECT team_id, slack_user_id, slack_display_name, slack_username,\n\t\t       slack_avatar_url, slack_is_bot, slack_deleted, now()\n\t\tFROM input\n\t\tON CONFLICT (team_id, slack_user_id) DO UPDATE\n\t\tSET slack_display_name = EXCLUDED.slack_display_name,\n\t\t    slack_username     = EXCLUDED.slack_username,\n\t\t    slack_avatar_url   = EXCLUDED.slack_avatar_url,\n\t\t    slack_is_bot       = EXCLUDED.slack_is_bot,\n\t\t    slack_deleted      = EXCLUDED.slack_deleted,\n\t\t    profile_updated_at = now()\n\t"
)

// Upsert writes a row with all the expected fields and clears revoked_at on
// conflict — re-linking after a revoke must resurrect the mapping.
func TestUpsert_WritesRow(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertQuery).
		WithArgs("T123", "U456", "user_abc", "org_xyz", "Acme", "acme", "https://avatars.slack-edge.com/icon.png", "alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Upsert(Mapping{
		TeamID: "T123", SlackUserID: "U456", WorkOSUserID: "user_abc",
		OrganizationID: "org_xyz",
		TeamName:       "Acme", TeamDomain: "acme", TeamIconURL: "https://avatars.slack-edge.com/icon.png",
		SlackUsername: "alice",
	})
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
		WithArgs("T1", "U1", "user_1", "", "", "", "", "").
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
			"organization_id",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}).
			AddRow("T1", "U1", "user_abc", "org_xyz", "Acme", "acme", "https://avatars.slack-edge.com/icon.png", "alice", now, now, nil).
			AddRow("T2", "U2", "user_abc", "", "", "", "", "", now, now, nil))

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
			"organization_id",
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
			"organization_id",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}).
			AddRow("T1", "U1", "user_a", "", "Acme", "acme", "", "alice", now, now, nil).
			AddRow("T2", "U2", "user_a", "", "Foo", "foo", "", "alice", now, now, nil).
			AddRow("T3", "U3", "user_b", "", "Bar", "bar", "", "bob", now, now, nil))

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
// PR 2 cutover: slack_observed_users is now the only write target;
// the legacy slack_identity_mappings dual-write is gone.
func TestUpsertObserved_FirstCallWrites(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedUserQuery).
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

	// Only one DB call expected: the new-table write on first call.
	// Second call short-circuits in the in-memory dedupe.
	mock.ExpectExec(upsertObservedUserQuery).
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

func TestUpsertObservedProfiles_BulkWritesUniqueDirectoryRows(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedProfilesQuery).
		WithArgs(
			pq.Array([]string{"T07XYZ", "T08XYZ"}),
			pq.Array([]string{"U07ABCDEF", "U08ABCDEF"}),
			pq.Array([]string{"Jesse Morgan", "Sohum Dalal"}),
			pq.Array([]string{"jesse", "sohum"}),
			pq.Array([]string{"https://avatars.slack-edge.com/jesse.png", "https://avatars.slack-edge.com/sohum.png"}),
			pq.Array([]bool{false, false}),
			pq.Array([]bool{false, true}),
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := store.UpsertObservedProfiles(t.Context(), []ObservedUser{
		{
			TeamID:      "T07XYZ",
			SlackUserID: "U07ABCDEF",
			Profile: SlackProfile{
				DisplayName: "Jesse Morgan",
				Username:    "jesse",
				AvatarURL:   "https://avatars.slack-edge.com/jesse.png",
			},
		},
		{
			TeamID:      "T07XYZ",
			SlackUserID: "U07ABCDEF",
			Profile: SlackProfile{
				DisplayName: "Duplicate Skipped",
			},
		},
		{TeamID: "", SlackUserID: "U-empty-team"},
		{TeamID: "T-empty-user", SlackUserID: ""},
		{
			TeamID:      "T08XYZ",
			SlackUserID: "U08ABCDEF",
			Profile: SlackProfile{
				DisplayName: "Sohum Dalal",
				Username:    "sohum",
				AvatarURL:   "https://avatars.slack-edge.com/sohum.png",
				Deleted:     true,
			},
		},
	}); err != nil {
		t.Fatalf("bulk upsert observed profiles: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("bulk profile upsert expectations: %v", err)
	}
}

func TestUpsertObservedProfiles_EmptyInputsAreNoOp(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	if err := store.UpsertObservedProfiles(t.Context(), nil); err != nil {
		t.Fatalf("nil input: %v", err)
	}
	if err := store.UpsertObservedProfiles(t.Context(), []ObservedUser{
		{TeamID: "", SlackUserID: "U07ABC"},
		{TeamID: "T07XYZ", SlackUserID: ""},
	}); err != nil {
		t.Fatalf("empty keys: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A repeat (team, user) pair after the dedupe map resets bumps
// last_seen_at via the ON CONFLICT DO UPDATE branch. PR 2 cutover: the
// new table has no revoked_at, no oauth/observed split; the only
// effect of a conflict is updating the timestamp. Revoke/revival logic
// for oauth identities lives on Upsert (the link flow), not here.
func TestUpsertObserved_ConflictBumpsLastSeen(t *testing.T) {
	store, mock, db := newMockStore(t)
	defer db.Close()

	mock.ExpectExec(upsertObservedUserQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row updated

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

	// Two DB calls expected: one before the reset, one after. The
	// dedupe map is wiped in between so the second call doesn't hit
	// the in-memory cache.
	mock.ExpectExec(upsertObservedUserQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(upsertObservedUserQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnResult(sqlmock.NewResult(0, 1))

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

	mock.ExpectExec(upsertObservedUserQuery).
		WithArgs("T07XYZ", "U07ABCDEF").
		WillReturnError(errors.New("temporary db hiccup"))
	mock.ExpectExec(upsertObservedUserQuery).
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

	// The query's `AND workos_user_id IS NOT NULL` filter excludes any
	// observed-only row that still has NULL workos_user_id during the
	// PR 3 transition window. Without the filter, Scan into a non-
	// nullable string would fail on a matched observed row. After the
	// schema migration restores the NOT NULL column constraint, the
	// filter is redundant but harmless.
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

func TestDirectoryEntriesForSlackUserIDs_ReturnsUniqueWorkspaceRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	store := NewStore(db)

	mock.ExpectQuery(`(?s)WITH input AS .*unambiguous AS`).
		WithArgs(pq.Array([]string{"U07CAROL00"})).
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id",
			"slack_user_id",
			"workos_user_id",
			"slack_display_name",
			"slack_username",
			"slack_avatar_url",
			"slack_is_bot",
			"slack_deleted",
			"team_name",
			"team_domain",
			"team_icon_url",
		}).AddRow(
			"T07POSTMAN",
			"U07CAROL00",
			"",
			"Carol Chen",
			"carol",
			"https://avatars.slack-edge.com/carol.png",
			false,
			false,
			"Postman",
			"postman",
			"https://avatars.slack-edge.com/postman.png",
		))

	out, err := store.DirectoryEntriesForSlackUserIDs([]string{"U07CAROL00", "U07CAROL00"})
	if err != nil {
		t.Fatalf("unscoped directory entries: %v", err)
	}
	entry := out["U07CAROL00"]
	if entry.TeamID != "T07POSTMAN" || entry.WorkspaceName != "Postman" {
		t.Errorf("entry mismatch: %+v", entry)
	}
	if entry.Profile.DisplayName != "Carol Chen" || entry.Profile.AvatarURL == "" {
		t.Errorf("profile mismatch: %+v", entry.Profile)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryEntriesForSlackUserIDs_EmptyInput(t *testing.T) {
	store, _, db := newMockStore(t)
	defer db.Close()

	out, err := store.DirectoryEntriesForSlackUserIDs(nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %d entries", len(out))
	}
}
