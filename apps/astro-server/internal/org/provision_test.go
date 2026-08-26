package org

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type fakeDirectory struct {
	existing     *Organization
	lookupErr    error
	created      []Organization
	createErr    error
	deleted      []string
	memberships  []Membership
	listErr      error
	createdRoles map[string]string
	membershipID string
	membershipEr error
}

func (f *fakeDirectory) GetOrganizationByExternalID(_ context.Context, externalID string) (Organization, error) {
	if f.lookupErr != nil {
		return Organization{}, f.lookupErr
	}
	if f.existing == nil {
		return Organization{}, ErrOrganizationNotFound
	}
	return Organization{ID: f.existing.ID, Name: f.existing.Name, ExternalID: externalID}, nil
}

func (f *fakeDirectory) CreateOrganization(_ context.Context, name, externalID string) (Organization, error) {
	if f.createErr != nil {
		return Organization{}, f.createErr
	}
	created := Organization{ID: "org_new", Name: name, ExternalID: externalID}
	f.created = append(f.created, created)
	return created, nil
}

func (f *fakeDirectory) DeleteOrganization(_ context.Context, workosOrgID string) error {
	f.deleted = append(f.deleted, workosOrgID)
	return nil
}

func (f *fakeDirectory) ListAllMemberships(_ context.Context, _ string) ([]Membership, error) {
	return f.memberships, f.listErr
}

func (f *fakeDirectory) CreateMembership(_ context.Context, workosOrgID, userID, roleSlug string) (Membership, error) {
	if f.membershipEr != nil {
		return Membership{}, f.membershipEr
	}
	if f.createdRoles == nil {
		f.createdRoles = map[string]string{}
	}
	f.createdRoles[userID] = roleSlug
	id := f.membershipID
	if id == "" {
		id = "om_new"
	}
	return Membership{ID: id, UserID: userID, OrganizationID: workosOrgID, RoleSlug: roleSlug}, nil
}

func provisionerFor(t *testing.T, dir directory) (*Provisioner, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Provisioner{
		directory: dir,
		accounts:  account.NewAccountStore(db),
		log:       logger.New("error", "json"),
	}, mock
}

func expectAccount(mock sqlmock.Sqlmock, id, name, accountType string, workosOrgID any) {
	mock.ExpectQuery("SELECT .+ FROM accounts a").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(id, name, accountType, workosOrgID, nil, time.Now(), time.Now())...))
}

func expectOwnerMembershipWrites(mock sqlmock.Sqlmock, accountID, owner string) {
	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow(owner))
	mock.ExpectExec("INSERT INTO account_members").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_member_workos").WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestEnsureOrganizationProvisionsPersonalAccount(t *testing.T) {
	dir := &fakeDirectory{}
	p, mock := provisionerFor(t, dir)

	expectAccount(mock, "acct-1", "saswat", "personal", nil)
	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-1", "org_new").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOwnerMembershipWrites(mock, "acct-1", "user-1")

	orgID, err := p.EnsureOrganization(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if orgID != "org_new" {
		t.Errorf("org id = %q, want org_new", orgID)
	}
	if len(dir.created) != 1 || dir.created[0].ExternalID != "acct-1" || dir.created[0].Name != "saswat" {
		t.Errorf("organization should carry the account id and handle: %+v", dir.created)
	}
	if dir.createdRoles["user-1"] != ownerRoleSlug {
		t.Errorf("owner role = %q, want %q", dir.createdRoles["user-1"], ownerRoleSlug)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestEnsureOrganizationAdoptsByExternalID(t *testing.T) {
	dir := &fakeDirectory{existing: &Organization{ID: "org_orphan", Name: "saswat"}}
	p, mock := provisionerFor(t, dir)

	expectAccount(mock, "acct-1", "saswat", "personal", nil)
	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-1", "org_orphan").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectOwnerMembershipWrites(mock, "acct-1", "user-1")

	orgID, err := p.EnsureOrganization(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if orgID != "org_orphan" {
		t.Errorf("org id = %q, want org_orphan", orgID)
	}
	if len(dir.created) != 0 {
		t.Errorf("adoption must not create a second organization: %+v", dir.created)
	}
}

func TestEnsureOrganizationKeepsExistingLinkAndMembership(t *testing.T) {
	dir := &fakeDirectory{memberships: []Membership{{ID: "om_1", UserID: "user-1", RoleSlug: "owner"}}}
	p, mock := provisionerFor(t, dir)

	expectAccount(mock, "acct-1", "saswat", "personal", "org_linked")
	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("user-1"))
	mock.ExpectExec("INSERT INTO account_members").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_member_workos").
		WithArgs("acct-1", "user-1", "om_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	orgID, err := p.EnsureOrganization(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if orgID != "org_linked" {
		t.Errorf("org id = %q, want org_linked", orgID)
	}
	if len(dir.created) != 0 || len(dir.createdRoles) != 0 {
		t.Error("a linked account with a membership needs no WorkOS writes")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestEnsureOrganizationFailsWhenOwnerMembershipFails(t *testing.T) {
	dir := &fakeDirectory{membershipEr: errors.New("workos down")}
	p, mock := provisionerFor(t, dir)

	expectAccount(mock, "acct-1", "acme", "organization", nil)
	mock.ExpectExec("INSERT INTO account_organizations").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("user-1"))

	if _, err := p.EnsureOrganization(context.Background(), "acct-1"); err == nil {
		t.Fatal("expected the membership failure to be returned")
	}
}

func TestDiscardOrganizationDeletesTheAdoptableOrganization(t *testing.T) {
	dir := &fakeDirectory{existing: &Organization{ID: "org_orphan"}}
	p, mock := provisionerFor(t, dir)

	expectAccount(mock, "acct-1", "acme", "organization", nil)

	p.DiscardOrganization(context.Background(), "acct-1")
	if len(dir.deleted) != 1 || dir.deleted[0] != "org_orphan" {
		t.Errorf("deleted = %v, want [org_orphan]", dir.deleted)
	}
}
