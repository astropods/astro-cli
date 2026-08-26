package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeploymentLifecycleWritePausesDuringAuthorizationMaintenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	store := NewDeploymentFGASyncStore(db, true).WithAuthorizationMaintenance()
	changed, err := store.RecordRegistrationTx(context.Background(), tx, "dep_123")
	if !errors.Is(err, ErrAuthorizationMaintenance) || changed {
		t.Fatalf("RecordRegistrationTx() = (%v, %v), want false, ErrAuthorizationMaintenance", changed, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccessIntentWritePausesDuringAuthorizationMaintenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	store := NewResourceAccessSyncStore(db).WithAuthorizationMaintenance()
	intent := AccessIntent{
		AccountID: "acct_123", OrganizationID: "org_123", Resource: DeploymentResource("dep_123"),
		Subject: MembershipAssignmentSubject("om_123"), SubjectID: "user_123", DesiredRole: RoleDeploymentViewer,
	}
	got, changed, err := store.Record(context.Background(), intent)
	if !errors.Is(err, ErrAuthorizationMaintenance) || changed || got != (AccessIntent{}) {
		t.Fatalf("Record() = (%+v, %v, %v), want zero intent, false, ErrAuthorizationMaintenance", got, changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
