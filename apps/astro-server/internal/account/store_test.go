package account

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetByWorkOSOrganizationID_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE workos_org_id").
		WithArgs("org_123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_123", now, now))

	acct, err := store.GetByWorkOSOrganizationID("org_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "acct-1" {
		t.Errorf("expected ID 'acct-1', got %q", acct.ID)
	}
	if acct.WorkOSOrganizationID != "org_123" {
		t.Errorf("expected workos_org_id 'org_123', got %q", acct.WorkOSOrganizationID)
	}
}

func TestGetByWorkOSOrganizationID_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts WHERE workos_org_id").
		WithArgs("org_unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}))

	_, err := store.GetByWorkOSOrganizationID("org_unknown")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestSetWorkOSOrganizationID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET workos_org_id").
		WithArgs("org_123", sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetWorkOSOrganizationID("acct-1", "org_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddMember_WithWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1", "admin", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.AddMember("acct-1", "user-1", "admin", "wm-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddMember_WithoutWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1", "member", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.AddMember("acct-1", "user-1", "member", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMember_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members WHERE account_id").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "owner", "wm-1", now))

	m, err := store.GetMember("acct-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Role != "owner" {
		t.Errorf("expected role 'owner', got %q", m.Role)
	}
	if m.WorkOSMembershipID != "wm-1" {
		t.Errorf("expected workos_membership_id 'wm-1', got %q", m.WorkOSMembershipID)
	}
}

func TestGetMember_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM account_members WHERE account_id").
		WithArgs("acct-1", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}))

	_, err := store.GetMember("acct-1", "user-2")
	if err == nil {
		t.Fatal("expected error for member not found")
	}
}

func TestGetMembersForAccount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members WHERE account_id").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "owner", "wm-1", now).
			AddRow("acct-1", "user-2", "admin", nil, now).
			AddRow("acct-1", "user-3", "member", "wm-3", now))

	members, err := store.GetMembersForAccount("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	if members[0].Role != "owner" {
		t.Errorf("first member should be owner, got %q", members[0].Role)
	}
	if members[1].WorkOSMembershipID != "" {
		t.Errorf("second member should have empty workos_membership_id, got %q", members[1].WorkOSMembershipID)
	}
	if members[2].WorkOSMembershipID != "wm-3" {
		t.Errorf("third member should have wm-3, got %q", members[2].WorkOSMembershipID)
	}
}

func TestGetMemberByWorkosMembershipID_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members WHERE workos_membership_id").
		WithArgs("wm-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "owner", "wm-1", now))

	m, err := store.GetMemberByWorkosMembershipID("wm-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", m.UserID)
	}
}

func TestGetMemberByWorkosMembershipID_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM account_members WHERE workos_membership_id").
		WithArgs("wm-unknown").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}))

	_, err := store.GetMemberByWorkosMembershipID("wm-unknown")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpsertMemberByWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members .+ ON CONFLICT").
		WithArgs("acct-1", "user-1", "admin", "wm-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.UpsertMemberByWorkosMembershipID("acct-1", "user-1", "admin", "wm-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMemberRole(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE account_members SET role").
		WithArgs("admin", "acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpdateMemberRole("acct-1", "user-1", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMemberRole_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE account_members SET role").
		WithArgs("admin", "acct-1", "user-2").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.UpdateMemberRole("acct-1", "user-2", "admin")
	if err == nil {
		t.Fatal("expected error for member not found")
	}
}

func TestRemoveMember(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("DELETE FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.RemoveMember("acct-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveMember_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("DELETE FROM account_members").
		WithArgs("acct-1", "user-2").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.RemoveMember("acct-1", "user-2")
	if err == nil {
		t.Fatal("expected error for member not found")
	}
}

func TestDeleteByID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("DELETE FROM accounts WHERE id").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.DeleteByID("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAccountsForUser_IncludesWorkOSOrgID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_members").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "role", "workos_org_id", "created_at", "updated_at"}).
			AddRow("acct-1", "personal", "personal", "owner", "", now, now).
			AddRow("acct-2", "myorg", "organization", "admin", "org_123", now, now))

	accounts, err := store.GetAccountsForUser("user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	if accounts[0].WorkOSOrganizationID != "" {
		t.Errorf("personal account should have empty workos_org_id, got %q", accounts[0].WorkOSOrganizationID)
	}
	if accounts[1].WorkOSOrganizationID != "org_123" {
		t.Errorf("org account should have workos_org_id 'org_123', got %q", accounts[1].WorkOSOrganizationID)
	}
}

func TestHasPersonalAccount_True(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM accounts a").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	has, err := store.HasPersonalAccount("user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected true, got false")
	}
}

func TestCountOwners_MultipleOwners(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := store.CountOwners("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestCountOwners_SingleOwner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	count, err := store.CountOwners("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestCountOwners_Zero(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	count, err := store.CountOwners("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestHasPersonalAccount_False(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM accounts a").
		WithArgs("user-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	has, err := store.HasPersonalAccount("user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false, got true")
	}
}
