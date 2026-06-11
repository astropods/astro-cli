package handlers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/lib/pq"
)

// Tests in this file cover the read-time identity hydration layer:
//   - newUserDetailsHydrator: partitions user_ids by kind, batches one
//     directory lookup per source (Slack / Astro), and stamps the
//     resolved fields back onto each row.
//   - Resolve*Identities: three call-site wrappers (users-summary,
//     account-summary cost-over-time, deployments-summary).
//
// The five user kinds the pipeline can encounter:
//   1. Astro user in-account
//   2. Astro user cross-account (still resolvable via personal-account
//      lookup; the join is global, not scoped to one account)
//   3. Slack user with directory entry (observed)
//   4. Slack user without directory entry (unknown to the directory)
//   5. Opaque id that's neither Slack-shaped nor WorkOS-prefixed
//
// Every test below asserts at least one of these kinds end-to-end so
// any future change to the hydrator's branching is caught.

// ── sqlmock helpers ─────────────────────────────────────────────────────

// newSlackStore returns a slackidentity.Store backed by sqlmock plus the
// mock for setting expectations. Cleanup is registered via t.Cleanup so
// each test doesn't have to remember to defer close.
func newSlackStore(t *testing.T) (*slackidentity.Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return slackidentity.NewStore(db), mock
}

// newAccountStore returns an account.AccountStore backed by sqlmock.
func newAccountStore(t *testing.T) (*account.AccountStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return account.NewAccountStore(db), mock
}

// slackRowCols matches the column shape DirectoryEntriesForSlackUserIDs
// scans. Stays in sync with slackidentity/store.go.
var slackRowCols = []string{
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
}

// expectSlackDirectoryQuery wires the typical DirectoryEntriesForSlackUserIDs
// expectation: matches the WITH-CTE query, takes a slice of slack_user_ids,
// and returns the rows the test wants stamped in. addRows handles 0-N rows.
func expectSlackDirectoryQuery(mock sqlmock.Sqlmock, ids []string, addRows func(rows *sqlmock.Rows)) {
	r := sqlmock.NewRows(slackRowCols)
	if addRows != nil {
		addRows(r)
	}
	mock.ExpectQuery(`(?s)WITH input AS .*unambiguous AS`).
		WithArgs(pq.Array(ids)).
		WillReturnRows(r)
}

// expectPersonalProfilesQuery wires the GetPersonalProfiles expectation.
// The query scans (user_id, name, display_name) for each WorkOS id.
func expectPersonalProfilesQuery(mock sqlmock.Sqlmock, ids []string, addRows func(rows *sqlmock.Rows)) {
	r := sqlmock.NewRows([]string{"user_id", "name", "display_name"})
	if addRows != nil {
		addRows(r)
	}
	mock.ExpectQuery(`(?s)SELECT am.user_id, a.name, a.display_name\s+FROM accounts a\s+JOIN account_members am`).
		WithArgs(pq.Array(ids)).
		WillReturnRows(r)
}

// noopLogger discards all log output so tests don't pollute output.
func noopLogger() *logger.Logger { return logger.New("error", "json") }

// ── hydrator unit tests ─────────────────────────────────────────────────

// Hydrator partitions ids by kind and runs at most one query per kind.
// Astro id batched against GetPersonalProfiles; Slack id batched against
// DirectoryEntriesForSlackUserIDs; unknown ids skip both lookups.
func TestUserDetailsHydrator_StampsAllKinds(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	h := newUserDetailsHydrator(
		noopLogger(),
		slackStore,
		accountStore,
		[]string{"user_01HXX_bob", "U07CAROL00", "anon-7f3"},
		"unit-test",
	)

	cases := []struct {
		uid    string
		input  UserDetails
		check  func(t *testing.T, got UserDetails)
	}{
		{
			uid:   "user_01HXX_bob",
			input: UserDetails{Kind: UserDetailsKindAstro},
			check: func(t *testing.T, got UserDetails) {
				if got.Kind != UserDetailsKindAstro {
					t.Errorf("kind=%v want astro", got.Kind)
				}
				if got.DisplayName != "Bob Smith" || got.Username != "bob" {
					t.Errorf("astro hydration mismatch: %+v", got)
				}
				if got.AvatarURL != "" || got.TeamID != "" {
					t.Errorf("astro row should not pick up slack fields: %+v", got)
				}
			},
		},
		{
			uid:   "U07CAROL00",
			input: UserDetails{Kind: UserDetailsKindSlack},
			check: func(t *testing.T, got UserDetails) {
				if got.Kind != UserDetailsKindSlack {
					t.Errorf("kind=%v want slack", got.Kind)
				}
				if got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" || got.AvatarURL == "" {
					t.Errorf("slack hydration mismatch: %+v", got)
				}
			},
		},
		{
			uid:   "anon-7f3",
			input: UserDetails{Kind: UserDetailsKindUnknown},
			check: func(t *testing.T, got UserDetails) {
				if got.Kind != UserDetailsKindUnknown {
					t.Errorf("kind=%v want unknown", got.Kind)
				}
				if got.DisplayName != "" || got.Username != "" || got.TeamID != "" {
					t.Errorf("unknown should stay empty: %+v", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.uid, func(t *testing.T) {
			got := tc.input
			h.stamp(tc.uid, &got)
			tc.check(t, got)
		})
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack expectations: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account expectations: %v", err)
	}
}

// Empty input set must short-circuit both lookups. Belt-and-suspenders:
// the mocks set no expectations, so any query at all would fail the test.
func TestUserDetailsHydrator_EmptyInputDoesNotQuery(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	h := newUserDetailsHydrator(noopLogger(), slackStore, accountStore, nil, "unit-test")
	if h == nil {
		t.Fatal("hydrator should be non-nil even with empty input")
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected slack query: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected account query: %v", err)
	}
}

// Single kind in the input set must only query the matching store —
// no Astro lookup when every id is Slack-shaped, and vice versa.
func TestUserDetailsHydrator_OnlyQueriesStoresForKindsPresent(t *testing.T) {
	t.Run("slack only", func(t *testing.T) {
		slackStore, slackMock := newSlackStore(t)
		accountStore, accountMock := newAccountStore(t)
		expectSlackDirectoryQuery(slackMock, []string{"U07ALICE00"}, nil)

		newUserDetailsHydrator(noopLogger(), slackStore, accountStore, []string{"U07ALICE00"}, "unit-test")

		if err := slackMock.ExpectationsWereMet(); err != nil {
			t.Errorf("slack: %v", err)
		}
		if err := accountMock.ExpectationsWereMet(); err != nil {
			t.Errorf("account should not have been queried: %v", err)
		}
	})

	t.Run("astro only", func(t *testing.T) {
		slackStore, slackMock := newSlackStore(t)
		accountStore, accountMock := newAccountStore(t)
		expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_alice"}, nil)

		newUserDetailsHydrator(noopLogger(), slackStore, accountStore, []string{"user_01HXX_alice"}, "unit-test")

		if err := slackMock.ExpectationsWereMet(); err != nil {
			t.Errorf("slack should not have been queried: %v", err)
		}
		if err := accountMock.ExpectationsWereMet(); err != nil {
			t.Errorf("account: %v", err)
		}
	})

	t.Run("unknown only", func(t *testing.T) {
		slackStore, slackMock := newSlackStore(t)
		accountStore, accountMock := newAccountStore(t)

		newUserDetailsHydrator(noopLogger(), slackStore, accountStore, []string{"anon-1", "anon-2"}, "unit-test")

		if err := slackMock.ExpectationsWereMet(); err != nil {
			t.Errorf("slack should not have been queried: %v", err)
		}
		if err := accountMock.ExpectationsWereMet(); err != nil {
			t.Errorf("account should not have been queried: %v", err)
		}
	})
}

// Lookup errors are non-fatal — the hydrator just leaves the row alone.
// The page degrades to raw user_ids instead of failing the request.
func TestUserDetailsHydrator_LookupErrorsAreNonFatal(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	slackMock.ExpectQuery(`(?s)WITH input AS`).
		WithArgs(pq.Array([]string{"U07CAROL00"})).
		WillReturnError(errors.New("slack store unavailable"))
	accountMock.ExpectQuery(`(?s)SELECT am.user_id, a.name`).
		WithArgs(pq.Array([]string{"user_01HXX_bob"})).
		WillReturnError(errors.New("account store unavailable"))

	h := newUserDetailsHydrator(
		noopLogger(),
		slackStore,
		accountStore,
		[]string{"user_01HXX_bob", "U07CAROL00"},
		"unit-test",
	)

	slack := UserDetails{Kind: UserDetailsKindSlack}
	h.stamp("U07CAROL00", &slack)
	if slack.DisplayName != "" || slack.TeamID != "" {
		t.Errorf("slack row should pass through on lookup error, got %+v", slack)
	}

	astro := UserDetails{Kind: UserDetailsKindAstro}
	h.stamp("user_01HXX_bob", &astro)
	if astro.DisplayName != "" || astro.Username != "" {
		t.Errorf("astro row should pass through on lookup error, got %+v", astro)
	}
}

// nil stores → no queries, no panics. Used to be a real code path before
// every handler started passing real stores; keep the guard tested so we
// don't regress.
func TestUserDetailsHydrator_NilStoresAreSafe(t *testing.T) {
	h := newUserDetailsHydrator(
		noopLogger(),
		nil, nil,
		[]string{"U07CAROL00", "user_01HXX_bob", "anon-x"},
		"unit-test",
	)
	if h == nil {
		t.Fatal("nil-store hydrator should not panic or return nil")
	}
	// Stamping should be a no-op — input details unchanged.
	cases := []struct {
		uid  string
		kind UserDetailsKind
	}{
		{"U07CAROL00", UserDetailsKindSlack},
		{"user_01HXX_bob", UserDetailsKindAstro},
		{"anon-x", UserDetailsKindUnknown},
	}
	for _, tc := range cases {
		got := UserDetails{Kind: tc.kind}
		h.stamp(tc.uid, &got)
		if got.DisplayName != "" || got.Username != "" || got.TeamID != "" || got.AvatarURL != "" {
			t.Errorf("%s: expected no stamping with nil stores, got %+v", tc.uid, got)
		}
	}
}

// Slack id present in the directory but the entry has no profile data
// (workspace not yet synced). The row keeps kind=slack and gets team_id
// but profile fields stay empty — the frontend renders the faceless
// "Slack user - U..." fallback.
func TestUserDetailsHydrator_SlackEntryWithoutProfile(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, _ := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07BARE00"}, func(r *sqlmock.Rows) {
		// Only team_id populated; everything else empty.
		r.AddRow("T07TEAM", "U07BARE00", "", "", "", "", false, false, "", "", "")
	})

	h := newUserDetailsHydrator(noopLogger(), slackStore, accountStore, []string{"U07BARE00"}, "unit-test")
	got := UserDetails{Kind: UserDetailsKindSlack}
	h.stamp("U07BARE00", &got)

	if got.Kind != UserDetailsKindSlack {
		t.Errorf("kind=%v want slack", got.Kind)
	}
	if got.TeamID != "T07TEAM" {
		t.Errorf("expected team_id, got %+v", got)
	}
	if got.DisplayName != "" || got.AvatarURL != "" {
		t.Errorf("empty-profile row should leave display fields blank, got %+v", got)
	}
}

// ── resolver tests ──────────────────────────────────────────────────────

// ResolveUsersSummaryIdentities stamps every row's user_details in
// place. A single users-summary response can carry all five kinds; the
// resolver should handle them in one pass with one Slack lookup and one
// Astro lookup. Pins the contract end-to-end with a real (mocked)
// directory + member store.
func TestResolveUsersSummaryIdentities_StampsAllKinds(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00", "U07GHOSTLY"}, func(r *sqlmock.Rows) {
		// Only Carol is in the directory; U07GHOSTLY is omitted (unknown).
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob", "user_01HXX_alice"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
		r.AddRow("user_01HXX_alice", "alice", "Alice Chen")
	})

	resp := &AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{
			// Astro user — name + username from personal profile lookup.
			{UserIdentity: UserIdentity{
				UserID:      "user_01HXX_bob",
				UserDetails: UserDetails{Kind: UserDetailsKindAstro},
			}, CostUSD: 10},
			// Cross-account astro user — lookup is global so it still hits.
			{UserIdentity: UserIdentity{
				UserID:      "user_01HXX_alice",
				UserDetails: UserDetails{Kind: UserDetailsKindAstro},
			}, CostUSD: 8},
			// Observed Slack — directory has profile + workspace.
			{UserIdentity: UserIdentity{
				UserID:      "U07CAROL00",
				UserDetails: UserDetails{Kind: UserDetailsKindSlack},
			}, CostUSD: 6},
			// Unknown Slack — directory misses, row stays raw.
			{UserIdentity: UserIdentity{
				UserID:      "U07GHOSTLY",
				UserDetails: UserDetails{Kind: UserDetailsKindSlack},
			}, CostUSD: 4},
			// Opaque id — neither shape; passes through.
			{UserIdentity: UserIdentity{
				UserID:      "anon-7f3",
				UserDetails: UserDetails{Kind: UserDetailsKindUnknown},
			}, CostUSD: 2},
		},
	}

	ResolveUsersSummaryIdentities(noopLogger(), slackStore, accountStore, resp)

	byID := make(map[string]UserDetails, len(resp.Users))
	for _, u := range resp.Users {
		byID[u.UserID] = u.UserDetails
	}

	if got := byID["user_01HXX_bob"]; got.Kind != UserDetailsKindAstro || got.DisplayName != "Bob Smith" || got.Username != "bob" {
		t.Errorf("in-account astro mismatch: %+v", got)
	}
	if got := byID["user_01HXX_alice"]; got.Kind != UserDetailsKindAstro || got.DisplayName != "Alice Chen" || got.Username != "alice" {
		t.Errorf("cross-account astro mismatch: %+v", got)
	}
	if got := byID["U07CAROL00"]; got.Kind != UserDetailsKindSlack || got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" || got.AvatarURL == "" {
		t.Errorf("observed slack mismatch: %+v", got)
	}
	if got := byID["U07GHOSTLY"]; got.Kind != UserDetailsKindSlack || got.DisplayName != "" || got.TeamID != "" {
		t.Errorf("unknown slack should be untouched: %+v", got)
	}
	if got := byID["anon-7f3"]; got.Kind != UserDetailsKindUnknown || got.DisplayName != "" {
		t.Errorf("opaque row should be untouched: %+v", got)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// ResolveAccountSummaryIdentities walks CostOverTimeByUser[].Users and
// stamps each row in place. Same ids may repeat across dates; the
// resolver dedupes the lookup batch but applies the stamp to every
// occurrence.
func TestResolveAccountSummaryIdentities_StampsCostOverTimeUsers(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	resp := &AccountObservabilitySummaryResponse{
		CostOverTimeByUser: []AccountCostOverTimeByUserEntry{
			{Date: "2026-06-09", Users: []AccountUserCost{
				{UserIdentity: UserIdentity{UserID: "user_01HXX_bob", UserDetails: UserDetails{Kind: UserDetailsKindAstro}}, CostUSD: 4},
				{UserIdentity: UserIdentity{UserID: "U07CAROL00", UserDetails: UserDetails{Kind: UserDetailsKindSlack}}, CostUSD: 2},
			}},
			{Date: "2026-06-10", Users: []AccountUserCost{
				// Same Bob, different day — must also receive the stamp.
				{UserIdentity: UserIdentity{UserID: "user_01HXX_bob", UserDetails: UserDetails{Kind: UserDetailsKindAstro}}, CostUSD: 6},
			}},
		},
	}

	ResolveAccountSummaryIdentities(noopLogger(), slackStore, accountStore, resp)

	day1 := resp.CostOverTimeByUser[0].Users
	if day1[0].UserDetails.Username != "bob" || day1[0].UserDetails.DisplayName != "Bob Smith" {
		t.Errorf("day-1 astro mismatch: %+v", day1[0].UserDetails)
	}
	if day1[1].UserDetails.TeamID != "T07POSTMAN" || day1[1].UserDetails.DisplayName != "Carol Chen" {
		t.Errorf("day-1 slack mismatch: %+v", day1[1].UserDetails)
	}
	day2 := resp.CostOverTimeByUser[1].Users
	if day2[0].UserDetails.Username != "bob" {
		t.Errorf("day-2 astro stamp missing: %+v", day2[0].UserDetails)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// ResolveDeploymentsSummaryIdentities walks each Deployment's
// UsersUsedDetails and stamps in place. Multiple deployments share
// users; the lookup batches across all of them.
func TestResolveDeploymentsSummaryIdentities_StampsUsersUsedDetails(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	resp := &AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{
			{
				DeploymentID: "dep-1", AgentName: "code-reviewer",
				UsersUsedDetails: []UserIdentity{
					{UserID: "user_01HXX_bob", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
					{UserID: "U07CAROL00", UserDetails: UserDetails{Kind: UserDetailsKindSlack}},
				},
			},
			{
				DeploymentID: "dep-2", AgentName: "swipefile",
				UsersUsedDetails: []UserIdentity{
					// Bob touches a second deployment — same Astro id, must
					// still get stamped.
					{UserID: "user_01HXX_bob", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
				},
			},
		},
	}

	ResolveDeploymentsSummaryIdentities(noopLogger(), slackStore, accountStore, resp)

	dep1 := resp.Deployments[0].UsersUsedDetails
	if dep1[0].UserDetails.Username != "bob" || dep1[0].UserDetails.DisplayName != "Bob Smith" {
		t.Errorf("dep-1 astro mismatch: %+v", dep1[0].UserDetails)
	}
	if dep1[1].UserDetails.TeamID != "T07POSTMAN" || dep1[1].UserDetails.DisplayName != "Carol Chen" {
		t.Errorf("dep-1 slack mismatch: %+v", dep1[1].UserDetails)
	}
	dep2 := resp.Deployments[1].UsersUsedDetails
	if dep2[0].UserDetails.Username != "bob" {
		t.Errorf("dep-2 astro stamp missing: %+v", dep2[0].UserDetails)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// ── Backward-compatibility tests ────────────────────────────────────────
//
// These tests cover the Redis-cache cutover at release time. Pre-deploy
// entries were written by the previous server build in the flat
// slack_* / identity_key shape; the new struct silently drops those
// fields during json.Unmarshal and leaves UserDetails{Kind: ""}. The
// backfill in hydrator.stamp() infers Kind from the user_id shape so
// the same read-time stamping pipeline applies to legacy entries and
// they render correctly on first read after deploy — no flush, no
// degraded window.
//
// The five legacy cases we need to keep working:
//   1. Legacy users-summary cached payload
//   2. Legacy cost_over_time_by_user payload (AccountUserCost rows)
//   3. Legacy users_used_details payload (deployment rows)
//   4. Direct hydrator.stamp() handles Kind="" via classification
//   5. The legacy fields don't leak through the marshal/respond path
//      (the new struct doesn't carry them, so they can't)

// Direct test of the backfill at the hydrator level. The previous tests
// drive end-to-end via the resolvers; this one pins the contract on
// hydrator.stamp() itself so any future refactor that strips the
// backfill is caught even if the resolver tests change.
func TestUserDetailsHydrator_Stamp_BackfillsEmptyKindByUserIDShape(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	h := newUserDetailsHydrator(
		noopLogger(),
		slackStore,
		accountStore,
		[]string{"user_01HXX_bob", "U07CAROL00", "anon-7f3"},
		"cutover-test",
	)

	// Every row starts with Kind="" — what a legacy cache entry looks
	// like after json.Unmarshal into the new struct.
	cases := []struct {
		uid       string
		wantKind  UserDetailsKind
		check     func(t *testing.T, got UserDetails)
	}{
		{
			uid:      "user_01HXX_bob",
			wantKind: UserDetailsKindAstro,
			check: func(t *testing.T, got UserDetails) {
				if got.DisplayName != "Bob Smith" || got.Username != "bob" {
					t.Errorf("astro stamp missing: %+v", got)
				}
			},
		},
		{
			uid:      "U07CAROL00",
			wantKind: UserDetailsKindSlack,
			check: func(t *testing.T, got UserDetails) {
				if got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" || got.AvatarURL == "" {
					t.Errorf("slack stamp missing: %+v", got)
				}
			},
		},
		{
			uid:      "anon-7f3",
			wantKind: UserDetailsKindUnknown,
			check: func(t *testing.T, got UserDetails) {
				if got.DisplayName != "" || got.TeamID != "" {
					t.Errorf("opaque row should be empty: %+v", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.uid, func(t *testing.T) {
			// Start with a zero-value UserDetails (Kind="") — the
			// shape json.Unmarshal of a legacy entry produces.
			got := UserDetails{}
			h.stamp(tc.uid, &got)
			if got.Kind != tc.wantKind {
				t.Errorf("kind=%v want %v", got.Kind, tc.wantKind)
			}
			tc.check(t, got)
		})
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// Cache-cutover safety net: the Redis cache will hold entries written
// by the pre-discriminated-union build of the server until the next
// refresh tick. Those bytes decode into the new struct as
// UserDetails{Kind: ""}. Without the backfill in stamp(), every cached
// row would render as Unknown user for up to 6h after deploy.
//
// This test simulates the cutover by hand-crafting JSON in the OLD
// wire format, unmarshaling it into the new struct, and running it
// through the resolver. The expected output: each row gets its kind
// populated from the user_id shape, the directory lookups fire, and
// the resolved fields land in user_details — exactly the shape a
// fresh-from-Langfuse response would have.
func TestResolveUsersSummaryIdentities_BackfillsKindForLegacyCacheEntries(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	// Old wire format: flat slack_* / identity_key fields, no user_details.
	// This is what a Redis entry written by the previous server build
	// looks like on disk.
	legacyBytes := []byte(`{
		"users": [
			{
				"identity_key": "user_01HXX_bob",
				"user_id": "user_01HXX_bob",
				"requests": 42,
				"cost_usd": 7.5,
				"tokens": 12400,
				"agents_used": []
			},
			{
				"identity_key": "slack:T07POSTMAN:U07CAROL00",
				"user_id": "U07CAROL00",
				"slack_team_id": "T07POSTMAN",
				"slack_display_name": "Carol Chen (stale)",
				"requests": 12,
				"cost_usd": 1.2,
				"tokens": 3100,
				"agents_used": []
			},
			{
				"identity_key": "anon-7f3",
				"user_id": "anon-7f3",
				"requests": 1,
				"cost_usd": 0.04,
				"tokens": 90,
				"agents_used": []
			}
		],
		"period": {"start": "", "end": "", "days": 0}
	}`)

	var resp AccountUsersSummaryResponse
	if err := json.Unmarshal(legacyBytes, &resp); err != nil {
		t.Fatalf("unmarshal legacy bytes: %v", err)
	}
	// Sanity: the legacy fields were silently dropped by the new struct.
	for _, u := range resp.Users {
		if u.UserDetails.Kind != "" {
			t.Fatalf("test premise broken: legacy unmarshal should leave Kind empty, got %q", u.UserDetails.Kind)
		}
	}

	// Directory + personal-profile lookups produce the same fresh data
	// they would for any live compute.
	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	ResolveUsersSummaryIdentities(noopLogger(), slackStore, accountStore, &resp)

	byID := make(map[string]UserDetails, len(resp.Users))
	for _, u := range resp.Users {
		byID[u.UserID] = u.UserDetails
	}

	if got := byID["user_01HXX_bob"]; got.Kind != UserDetailsKindAstro || got.DisplayName != "Bob Smith" || got.Username != "bob" {
		t.Errorf("legacy astro row not backfilled correctly: %+v", got)
	}
	if got := byID["U07CAROL00"]; got.Kind != UserDetailsKindSlack || got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" {
		t.Errorf("legacy slack row not backfilled correctly: %+v", got)
	}
	if got := byID["anon-7f3"]; got.Kind != UserDetailsKindUnknown {
		t.Errorf("legacy opaque row should classify as unknown, got %+v", got)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// Same scenario for the account-summary cost_over_time_by_user shape.
// Legacy entries had identity_key + slack_team_id on each per-(day,
// user) row. The new struct treats them as unknown fields, so each
// AccountUserCost decodes with UserIdentity{UserDetails:{Kind:""}}.
// The resolver back-fills + stamps.
func TestResolveAccountSummaryIdentities_BackfillsKindForLegacyCacheEntries(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	legacyBytes := []byte(`{
		"period": {"start": "", "end": "", "days": 0},
		"totals": {"cost_usd": 0, "requests": 0, "input_tokens": 0, "output_tokens": 0, "total_tokens": 0, "active_agents": 0},
		"daily_avg": {"cost_usd": 0, "requests": 0, "tokens": 0},
		"cost_over_time": [],
		"cost_by_model": [],
		"sparklines": {"cost": [], "requests": [], "tokens": []},
		"cost_over_time_by_user": [
			{
				"date": "2026-06-09",
				"users": [
					{"identity_key": "user_01HXX_bob", "user_id": "user_01HXX_bob", "cost_usd": 4, "requests": 10, "tokens": 200},
					{"identity_key": "slack:T07POSTMAN:U07CAROL00", "user_id": "U07CAROL00", "slack_team_id": "T07POSTMAN", "cost_usd": 2, "requests": 5, "tokens": 100}
				]
			}
		]
	}`)

	var resp AccountObservabilitySummaryResponse
	if err := json.Unmarshal(legacyBytes, &resp); err != nil {
		t.Fatalf("unmarshal legacy bytes: %v", err)
	}
	for _, e := range resp.CostOverTimeByUser {
		for _, u := range e.Users {
			if u.UserDetails.Kind != "" {
				t.Fatalf("test premise broken: kind should be empty post-unmarshal, got %q", u.UserDetails.Kind)
			}
		}
	}

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	ResolveAccountSummaryIdentities(noopLogger(), slackStore, accountStore, &resp)

	users := resp.CostOverTimeByUser[0].Users
	byID := make(map[string]UserDetails, len(users))
	for _, u := range users {
		byID[u.UserID] = u.UserDetails
	}
	if got := byID["user_01HXX_bob"]; got.Kind != UserDetailsKindAstro || got.DisplayName != "Bob Smith" {
		t.Errorf("legacy astro cost-over-time row not backfilled: %+v", got)
	}
	if got := byID["U07CAROL00"]; got.Kind != UserDetailsKindSlack || got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" {
		t.Errorf("legacy slack cost-over-time row not backfilled: %+v", got)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// Same scenario for the deployments-summary users_used_details shape.
// Legacy entries were []UserIdentityRef with flat slack_* fields; the
// new struct is []UserIdentity with nested user_details. Unmarshal
// drops the legacy fields, leaving Kind="" on each entry; the resolver
// back-fills + stamps.
func TestResolveDeploymentsSummaryIdentities_BackfillsKindForLegacyCacheEntries(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	legacyBytes := []byte(`{
		"period": {"start": "", "end": "", "days": 0},
		"deployments": [
			{
				"deployment_id": "dep-1",
				"agent_name": "code-reviewer",
				"requests": 0, "cost_usd": 0, "cost_per_request": 0,
				"input_tokens": 0, "output_tokens": 0, "total_tokens": 0,
				"tok_per_request": 0, "p95_latency_ms": 0, "top_model": "",
				"users_used": ["user_01HXX_bob", "U07CAROL00"],
				"users_used_details": [
					{"identity_key": "user_01HXX_bob", "user_id": "user_01HXX_bob"},
					{"identity_key": "slack:T07POSTMAN:U07CAROL00", "user_id": "U07CAROL00", "slack_team_id": "T07POSTMAN", "slack_display_name": "Carol Chen (stale)"}
				]
			}
		]
	}`)

	var resp AccountDeploymentsSummaryResponse
	if err := json.Unmarshal(legacyBytes, &resp); err != nil {
		t.Fatalf("unmarshal legacy bytes: %v", err)
	}
	for _, d := range resp.Deployments {
		for _, u := range d.UsersUsedDetails {
			if u.UserDetails.Kind != "" {
				t.Fatalf("test premise broken: kind should be empty post-unmarshal, got %q", u.UserDetails.Kind)
			}
		}
	}

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/carol.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})
	expectPersonalProfilesQuery(accountMock, []string{"user_01HXX_bob"}, func(r *sqlmock.Rows) {
		r.AddRow("user_01HXX_bob", "bob", "Bob Smith")
	})

	ResolveDeploymentsSummaryIdentities(noopLogger(), slackStore, accountStore, &resp)

	details := resp.Deployments[0].UsersUsedDetails
	byID := make(map[string]UserDetails, len(details))
	for _, d := range details {
		byID[d.UserID] = d.UserDetails
	}
	if got := byID["user_01HXX_bob"]; got.Kind != UserDetailsKindAstro || got.Username != "bob" {
		t.Errorf("legacy astro users_used_details not backfilled: %+v", got)
	}
	if got := byID["U07CAROL00"]; got.Kind != UserDetailsKindSlack || got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" {
		t.Errorf("legacy slack users_used_details not backfilled: %+v", got)
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// Stale slack_display_name baked into a legacy cache entry must NOT
// leak into the post-resolution response — the new struct silently
// drops the legacy field during unmarshal, and the resolver writes the
// fresh directory value into user_details.display_name. Pins the
// promise that read-time stamping is the single source of truth.
func TestResolveUsersSummaryIdentities_LegacyStaleFieldsDoNotLeakThrough(t *testing.T) {
	slackStore, slackMock := newSlackStore(t)
	accountStore, accountMock := newAccountStore(t)

	legacyBytes := []byte(`{
		"users": [{
			"identity_key": "slack:T07POSTMAN:U07CAROL00",
			"user_id": "U07CAROL00",
			"slack_team_id": "T07POSTMAN",
			"slack_display_name": "OLD NAME",
			"slack_username": "old-handle",
			"slack_avatar_url": "https://avatars.slack-edge.com/STALE.png",
			"requests": 1,
			"cost_usd": 0,
			"tokens": 0,
			"agents_used": []
		}],
		"period": {"start": "", "end": "", "days": 0}
	}`)

	var resp AccountUsersSummaryResponse
	if err := json.Unmarshal(legacyBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expectSlackDirectoryQuery(slackMock, []string{"U07CAROL00"}, func(r *sqlmock.Rows) {
		// Directory now has the FRESH values — Carol renamed herself.
		r.AddRow("T07POSTMAN", "U07CAROL00", "",
			"Carol Chen", "carol", "https://avatars.slack-edge.com/FRESH.png",
			false, false, "Postman", "postman", "https://slack-edge.com/postman.png")
	})

	ResolveUsersSummaryIdentities(noopLogger(), slackStore, accountStore, &resp)

	got := resp.Users[0].UserDetails
	if got.DisplayName != "Carol Chen" || got.AvatarURL != "https://avatars.slack-edge.com/FRESH.png" || got.Username != "carol" {
		t.Errorf("stale legacy fields leaked or fresh stamp missing: %+v", got)
	}

	// Roundtrip through JSON to verify the on-wire shape matches the
	// new contract — no slack_display_name / identity_key / etc.
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	wire := string(out)
	for _, banned := range []string{
		`"slack_display_name"`,
		`"slack_username"`,
		`"slack_avatar_url"`,
		`"slack_team_id"`,
		`"identity_key"`,
	} {
		if strings.Contains(wire, banned) {
			t.Errorf("legacy field %s leaked into response wire format: %s", banned, wire)
		}
	}

	if err := slackMock.ExpectationsWereMet(); err != nil {
		t.Errorf("slack: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account: %v", err)
	}
}

// Resolver no-ops cleanly with a nil response or an empty user list —
// don't query the stores. The handler path could pass nil after a
// degraded response, so the guard matters.
func TestResolvers_NoQueryOnNilOrEmpty(t *testing.T) {
	t.Run("nil users-summary", func(t *testing.T) {
		slackStore, slackMock := newSlackStore(t)
		accountStore, accountMock := newAccountStore(t)
		ResolveUsersSummaryIdentities(noopLogger(), slackStore, accountStore, nil)
		if err := slackMock.ExpectationsWereMet(); err != nil {
			t.Errorf("unexpected slack query: %v", err)
		}
		if err := accountMock.ExpectationsWereMet(); err != nil {
			t.Errorf("unexpected account query: %v", err)
		}
	})
	t.Run("empty users-summary", func(t *testing.T) {
		slackStore, slackMock := newSlackStore(t)
		accountStore, accountMock := newAccountStore(t)
		ResolveUsersSummaryIdentities(noopLogger(), slackStore, accountStore, &AccountUsersSummaryResponse{})
		if err := slackMock.ExpectationsWereMet(); err != nil {
			t.Errorf("unexpected slack query: %v", err)
		}
		if err := accountMock.ExpectationsWereMet(); err != nil {
			t.Errorf("unexpected account query: %v", err)
		}
	})
}
