package org

import (
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
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

// Both guards run before any WorkOS call, so a nil client is the assertion: if
// the guard let the request through, the test would panic instead of failing.

func TestSyncMembershipsForUser_SkipsUserWithNoPersonalAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	sync := NewSync(nil, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	if _, err := sync.SyncMembershipsForUser(context.Background(), "user-1"); err != nil {
		t.Fatalf("expected the sync to skip cleanly, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSyncMembershipsForUser_IdentityCheckFailureIsFatal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
		WithArgs("user-1").
		WillReturnError(context.DeadlineExceeded)

	sync := NewSync(nil, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	if _, err := sync.SyncMembershipsForUser(context.Background(), "user-1"); err == nil {
		t.Fatal("expected an error when the identity check fails")
	}
}

func TestAddMember_RejectsUserWithNoPersonalAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT .+ FROM accounts a").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).AddRow(
			"acct-1", "myorg", "organization", "wos-org-1", nil, time.Now(), time.Now(),
			"My Org", nil, nil,
			nil, nil, nil, nil, nil, nil,
			"{}", "{}",
		))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	sync := NewSync(nil, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	_, err = sync.AddMember(context.Background(), "acct-1", "user-1", "member")
	if err == nil {
		t.Fatal("expected AddMember to reject a user with no personal account")
	}
	if !strings.Contains(err.Error(), "account setup") {
		t.Errorf("expected a setup-incomplete error, got %v", err)
	}
}

func TestSyncMembershipsForUser_PromotesOwnerBelowAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .+ FROM accounts a").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).AddRow(
			"acct-1", "myorg", "organization", "org_1", nil, time.Now(), time.Now(),
			"My Org", nil, nil,
			nil, nil, nil, nil, nil, nil,
			"{}", "{}",
		))
	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_member_workos").
		WithArgs("acct-1", "user-1", "om_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("user-1"))

	var updated []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var body struct {
				RoleSlug string `json:"role_slug"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode role update: %v", err)
			}
			updated = append(updated, r.URL.Path+" "+body.RoleSlug)
			fmt.Fprint(w, `{"id":"om_1","user_id":"user-1","organization_id":"org_1","role":{"slug":"admin"},"status":"active"}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"om_1","user_id":"user-1","organization_id":"org_1","role":{"slug":"member"},"status":"active"}],"list_metadata":{"before":"","after":""}}`)
	}))
	defer srv.Close()

	client := &Client{um: &usermanagement.Client{
		APIKey:     "sk_test",
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
		JSONEncode: json.Marshal,
	}}
	sync := NewSync(client, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	repaired, err := sync.SyncMembershipsForUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("SyncMembershipsForUser: %v", err)
	}
	if len(repaired) != 1 || repaired[0] != "org_1" {
		t.Fatalf("repaired orgs = %v, want the caller told which token to re-issue", repaired)
	}
	want := "/user_management/organization_memberships/om_1 admin"
	if len(updated) != 1 || updated[0] != want {
		t.Fatalf("role updates = %v, want the owner promoted to admin", updated)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A nil client is the assertion for both cases below: reaching WorkOS panics.

func TestRepairOwnerRole_SkipsRolesThatAlreadyAdminister(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	sync := NewSync(nil, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	for _, slug := range []string{"admin", "owner"} {
		sync.repairOwnerRole(context.Background(), "acct-1", Membership{
			ID: "om_1", UserID: "user-1", RoleSlug: slug,
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRepairOwnerRole_LeavesNonOwnersAlone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("user-2"))

	sync := NewSync(nil, account.NewAccountStore(db), nil, db, logger.New("error", "json"))

	sync.repairOwnerRole(context.Background(), "acct-1", Membership{
		ID: "om_1", UserID: "user-1", RoleSlug: "member",
	})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

type fakeRoleIntents struct {
	recorded []authz.AccessIntent
}

func (f *fakeRoleIntents) Record(_ context.Context, intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
	f.recorded = append(f.recorded, intent)
	intent.DesiredVersion = 1
	return intent, true, nil
}

type fakeRoleMembers struct{}

func (fakeRoleMembers) GetMemberContext(context.Context, string, string) (*account.AccountMember, error) {
	return nil, nil
}

type fakeRoleQueue struct{}

func (fakeRoleQueue) InsertResourceAccessFGAReconcileJob(context.Context, authz.AccessIntentKey) error {
	return nil
}

// Login is where an invited member's membership first appears locally, so it is
// also where their account role has to be projected.
func TestSyncMembershipsForUser_ProjectsAccountRole(t *testing.T) {
	for _, tc := range []struct {
		name           string
		membershipRole string
		ownerUserID    string
		want           authz.RoleSlug
	}{
		{"ordinary member", "member", "user-2", authz.RoleAccountMember},
		{"owner repaired to admin", "member", "user-1", authz.RoleAccountAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close() //nolint:errcheck

			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
				WithArgs("user-1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT .+ FROM accounts a").
				WithArgs("org_1").
				WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).AddRow(
					"acct-1", "myorg", "organization", "org_1", nil, time.Now(), time.Now(),
					"My Org", nil, nil,
					nil, nil, nil, nil, nil, nil,
					"{}", "{}",
				))
			mock.ExpectExec("INSERT INTO account_members").
				WithArgs("acct-1", "user-1").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO account_member_workos").
				WithArgs("acct-1", "user-1", "om_1").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT owner_user_id FROM accounts").
				WithArgs("acct-1").
				WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow(tc.ownerUserID))

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPut {
					fmt.Fprint(w, `{"id":"om_1","user_id":"user-1","organization_id":"org_1","role":{"slug":"admin"},"status":"active"}`)
					return
				}
				fmt.Fprintf(w, `{"data":[{"id":"om_1","user_id":"user-1","organization_id":"org_1","role":{"slug":%q},"status":"active"}],"list_metadata":{"before":"","after":""}}`, tc.membershipRole)
			}))
			defer srv.Close()

			client := &Client{um: &usermanagement.Client{
				APIKey:     "sk_test",
				Endpoint:   srv.URL,
				HTTPClient: srv.Client(),
				JSONEncode: json.Marshal,
			}}
			intents := &fakeRoleIntents{}
			sync := NewSync(client, account.NewAccountStore(db), nil, db, logger.New("error", "json"))
			sync.SetAccountRoles(authz.NewRoleProjector(intents, fakeRoleMembers{}, fakeRoleQueue{}))

			if _, err := sync.SyncMembershipsForUser(context.Background(), "user-1"); err != nil {
				t.Fatalf("SyncMembershipsForUser: %v", err)
			}
			if len(intents.recorded) != 1 {
				t.Fatalf("recorded intents = %+v", intents.recorded)
			}
			intent := intents.recorded[0]
			if intent.DesiredRole != tc.want {
				t.Fatalf("desired role = %q, want %q", intent.DesiredRole, tc.want)
			}
			if intent.Resource != authz.AccountResource("acct-1") {
				t.Fatalf("resource = %+v, want the Account resource", intent.Resource)
			}
			if intent.Subject != authz.MembershipAssignmentSubject("om_1") || intent.SubjectID != "user-1" {
				t.Fatalf("subject = %+v/%q", intent.Subject, intent.SubjectID)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}
