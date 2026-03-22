package account

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreate_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO accounts").
		WithArgs("myorg", "organization", "My Org", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "display_name", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "My Org", time.Now(), time.Now()))
	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	acct, err := store.Create("myorg", "organization", "user-1", "My Org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "acct-1" {
		t.Errorf("expected ID 'acct-1', got %q", acct.ID)
	}
	if acct.Name != "myorg" {
		t.Errorf("expected name 'myorg', got %q", acct.Name)
	}
	if acct.WorkOSOrganizationID != "" {
		t.Errorf("expected empty workos_org_id on create, got %q", acct.WorkOSOrganizationID)
	}
}

func TestCreate_InvalidName(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := NewAccountStore(db)

	_, err := store.Create("ab", "personal", "user-1", "")
	if err == nil {
		t.Fatal("expected error for short name")
	}
}

func TestGetByName_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, now, now, 0, ""))

	acct, err := store.GetByName("myorg")
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

func TestGetByName_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}))

	_, err := store.GetByName("unknown")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetByName_PersonalAccount_NullWorkOSOrgID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("personal").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}).
			AddRow("acct-1", "personal", "personal", nil, nil, now, now, 0, ""))

	acct, err := store.GetByName("personal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.WorkOSOrganizationID != "" {
		t.Errorf("personal account should have empty workos_org_id, got %q", acct.WorkOSOrganizationID)
	}
}

func TestGetByID_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, now, now, 0, ""))

	acct, err := store.GetByID("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.Name != "myorg" {
		t.Errorf("expected name 'myorg', got %q", acct.Name)
	}
	if acct.WorkOSOrganizationID != "org_123" {
		t.Errorf("expected workos_org_id 'org_123', got %q", acct.WorkOSOrganizationID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("unknown-id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}))

	_, err := store.GetByID("unknown-id")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetByWorkOSOrganizationID_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, now, now, 0, ""))

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

	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version", "display_name"}))

	_, err := store.GetByWorkOSOrganizationID("org_unknown")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestSetWorkOSOrganizationID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-1", "org_123").
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
		WithArgs("acct-1", "user-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO account_member_workos").
		WithArgs("acct-1", "user-1", "wm-123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.AddMember("acct-1", "user-1", "wm-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddMember_WithoutWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.AddMember("acct-1", "user-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMember_NullWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", nil, now))

	m, err := store.GetMember("acct-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.WorkOSMembershipID != "" {
		t.Errorf("expected empty workos_membership_id, got %q", m.WorkOSMembershipID)
	}
}

func TestGetMember_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", now))

	m, err := store.GetMember("acct-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.WorkOSMembershipID != "wm-1" {
		t.Errorf("expected workos_membership_id 'wm-1', got %q", m.WorkOSMembershipID)
	}
}

func TestAddMember_WorkosLinkInsertFailure(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members").
		WithArgs("acct-1", "user-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO account_member_workos").
		WithArgs("acct-1", "user-1", "wm-dup").
		WillReturnError(sqlmock.ErrCancelled)

	err := store.AddMember("acct-1", "user-1", "wm-dup")
	if err == nil {
		t.Fatal("expected error when workos link insert fails")
	}
}

func TestUpsertMemberByWorkosMembershipID_WorkosLinkFailure(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members .+ ON CONFLICT").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO account_member_workos .+ ON CONFLICT").
		WithArgs("acct-1", "user-1", "wm-1").
		WillReturnError(sqlmock.ErrCancelled)

	err := store.UpsertMemberByWorkosMembershipID("acct-1", "user-1", "wm-1")
	if err == nil {
		t.Fatal("expected error when workos link upsert fails")
	}
}

func TestGetMember_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}))

	_, err := store.GetMember("acct-1", "user-2")
	if err == nil {
		t.Fatal("expected error for member not found")
	}
}

func TestGetMembersForAccount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", now).
			AddRow("acct-1", "user-2", nil, now).
			AddRow("acct-1", "user-3", "wm-3", now))

	members, err := store.GetMembersForAccount("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
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
	mock.ExpectQuery("SELECT .+ FROM account_member_workos mw JOIN account_members am").
		WithArgs("wm-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", now))

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

	mock.ExpectQuery("SELECT .+ FROM account_member_workos mw JOIN account_members am").
		WithArgs("wm-unknown").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}))

	_, err := store.GetMemberByWorkosMembershipID("wm-unknown")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpsertMemberByWorkosMembershipID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("INSERT INTO account_members .+ ON CONFLICT").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO account_member_workos .+ ON CONFLICT").
		WithArgs("acct-1", "user-1", "wm-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.UpsertMemberByWorkosMembershipID("acct-1", "user-1", "wm-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestSetOpenMeterCustomerID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET openmeter_customer_id").
		WithArgs("om-cust-1", sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetOpenMeterCustomerID("acct-1", "om-cust-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAccountsMissingOpenMeterCustomer(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "org1", "organization", now, now).
			AddRow("acct-2", "personal1", "personal", now, now))

	accounts, err := store.GetAccountsMissingOpenMeterCustomer(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].ID != "acct-1" {
		t.Errorf("expected 'acct-1', got %q", accounts[0].ID)
	}
}

func TestGetAccountsMissingOpenMeterCustomer_Empty(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}))

	accounts, err := store.GetAccountsMissingOpenMeterCustomer(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestCreateWithoutOwner_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectQuery("INSERT INTO accounts").
		WithArgs("external-org", "organization", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "external-org", "organization", time.Now(), time.Now()))

	acct, err := store.CreateWithoutOwner("external-org", "organization")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "acct-1" {
		t.Errorf("expected ID 'acct-1', got %q", acct.ID)
	}
}

func TestCreateWithoutOwner_InvalidName(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := NewAccountStore(db)

	_, err := store.CreateWithoutOwner("ab", "organization")
	if err == nil {
		t.Fatal("expected error for short name")
	}
}

func TestRemoveUserFromAllAccounts(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("DELETE FROM account_members WHERE user_id").
		WithArgs("user-1").
		WillReturnResult(sqlmock.NewResult(0, 3))

	n, err := store.RemoveUserFromAllAccounts("user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 rows affected, got %d", n)
	}
}

func TestRemoveUserFromAllAccounts_NoRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("DELETE FROM account_members WHERE user_id").
		WithArgs("user-gone").
		WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := store.RemoveUserFromAllAccounts("user-gone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows affected, got %d", n)
	}
}

func TestMarkDeleted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET deleted_at").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.MarkDeleted("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkDeleted_AlreadyDeleted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET deleted_at").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.MarkDeleted("acct-1")
	if err == nil {
		t.Fatal("expected error for already deleted account")
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
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_members am .+ LEFT JOIN account_organizations ao").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at", "avatar_version", "display_name"}).
			AddRow("acct-1", "personal", "personal", "", now, now, 0, "").
			AddRow("acct-2", "myorg", "organization", "org_123", now, now, 0, ""))

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
