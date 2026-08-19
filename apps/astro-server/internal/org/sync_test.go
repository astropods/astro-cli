package org

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
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

	sync := NewSync(nil, account.NewAccountStore(db), nil, db)

	if err := sync.SyncMembershipsForUser(context.Background(), "user-1"); err != nil {
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

	sync := NewSync(nil, account.NewAccountStore(db), nil, db)

	if err := sync.SyncMembershipsForUser(context.Background(), "user-1"); err == nil {
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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
			"display_name", "avatar_colors", "avatar_updated_at", "cluster_id",
			"account_number", "bio", "location", "local_timezone", "pronouns", "website",
			"social_links", "blueprint_order",
		}).AddRow(
			"acct-1", "myorg", "organization", "wos-org-1", nil, time.Now(), time.Now(),
			"My Org", nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			"{}", "{}",
		))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts a").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	sync := NewSync(nil, account.NewAccountStore(db), nil, db)

	_, err = sync.AddMember(context.Background(), "acct-1", "user-1", "member")
	if err == nil {
		t.Fatal("expected AddMember to reject a user with no personal account")
	}
	if !strings.Contains(err.Error(), "account setup") {
		t.Errorf("expected a setup-incomplete error, got %v", err)
	}
}
