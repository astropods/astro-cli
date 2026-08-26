package authz

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAuthorizationResourceSyncRecordsBlueprintAndAccountParent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)SELECT a.account_id.*FROM agents a`).
		WithArgs("acct-1", "coach").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "workos_org_id", "uid", "name", "created_by"}).
			AddRow("acct-1", "org-1", "blueprint-1", "coach", "user-1"))
	mock.ExpectQuery(`(?s)SELECT a.id.*FROM accounts a`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "workos_org_id", "name", "owner_user_id"}).
			AddRow("acct-1", "org-1", "Astro Spaceship", "owner-1"))
	mock.ExpectExec(`INSERT INTO authorization_resource_sync`).
		WithArgs("acct-1", "org-1", ResourceAccount, "acct-1", ResourceOrganization, "org-1", "Astro Spaceship", "owner-1", RoleAccountAdmin).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO authorization_resource_sync`).
		WithArgs("acct-1", "org-1", ResourceBlueprint, "blueprint-1", ResourceAccount, "acct-1", "coach", "user-1", RoleBlueprintAdmin).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewAuthorizationResourceSyncStore(db, true)
	key, changed, err := store.RecordBlueprintRegistrationTx(context.Background(), tx, "acct-1", "coach")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || key.OrganizationID != "org-1" || key.Resource != BlueprintResource("blueprint-1") {
		t.Fatalf("registration = (%+v, %t)", key, changed)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationResourceSyncRecordsDeploymentOnlyInGenericLedger(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)SELECT d.account_id.*FROM deployments d`).
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "workos_org_id", "id", "name", "deployed_by"}).
			AddRow("acct-1", "org-1", "dep-1", "Coach", "user-1"))
	mock.ExpectQuery(`(?s)SELECT a.id.*FROM accounts a`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "workos_org_id", "name", "owner_user_id"}).
			AddRow("acct-1", "org-1", "Astro Spaceship", "owner-1"))
	mock.ExpectExec(`INSERT INTO authorization_resource_sync`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO authorization_resource_sync`).WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewAuthorizationResourceSyncStore(db, true)
	changed, err := store.RecordRegistrationTx(context.Background(), tx, "dep-1")
	if err != nil || !changed {
		t.Fatalf("RecordRegistrationTx() = (%t, %v)", changed, err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
