package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeploymentFGASyncStoreRecordState(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`(?s)INSERT INTO deployment_fga_sync.*a\.type = 'organization'`).
		WithArgs("dep_123", DeploymentFGARegistered).
		WillReturnResult(sqlmock.NewResult(0, 1))
	recorded, err := NewDeploymentFGASyncStore(db, true).RecordRegistrationTx(context.Background(), tx, "dep_123")
	if err != nil || !recorded {
		t.Fatalf("RecordRegistrationTx() = %v, %v; want true, nil", recorded, err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentFGASyncStoreRecordNameUpdate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*desired_version = desired_version \+ 1.*desired_state = 'registered'`).
		WithArgs("dep_123").
		WillReturnResult(sqlmock.NewResult(0, 1))
	recorded, err := NewDeploymentFGASyncStore(db, true).RecordNameUpdateTx(context.Background(), tx, "dep_123")
	if err != nil || !recorded {
		t.Fatalf("RecordNameUpdateTx() = %v, %v; want true, nil", recorded, err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentFGASyncStoreRecordStateValidation(t *testing.T) {
	store := NewDeploymentFGASyncStore(nil, true)
	if _, err := store.recordStateTx(context.Background(), nil, "dep_123", DeploymentFGARegistered); err != nil {
		t.Fatalf("disabled store should no-op, got %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	store = NewDeploymentFGASyncStore(db, true)
	if _, err := store.recordStateTx(context.Background(), nil, "dep_123", DeploymentFGARegistered); err == nil {
		t.Fatal("nil transaction should fail")
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := store.recordStateTx(context.Background(), tx, "", DeploymentFGARegistered); err == nil {
		t.Fatal("empty deployment id should fail")
	}
}

func TestDeploymentFGASyncStoreUnconfiguredMutations(t *testing.T) {
	var store *DeploymentFGASyncStore
	if _, err := store.MarkSynced(context.Background(), "dep_123", DeploymentFGARegistered, 1); err == nil {
		t.Fatal("MarkSynced() should fail for an unconfigured store")
	}
	if _, err := store.DeferCreatorAssignment(context.Background(), "dep_123", DeploymentFGARegistered, 1); err == nil {
		t.Fatal("DeferCreatorAssignment() should fail for an unconfigured store")
	}
	if err := store.RecordFailure(context.Background(), "dep_123", DeploymentFGARegistered, 1, errors.New("failed")); err == nil {
		t.Fatal("RecordFailure() should fail for an unconfigured store")
	}
}

func TestDeploymentFGASyncStorePending(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`(?s)SELECT s\.deployment_id.*OR s\.creator_assignment_pending`).
		WithArgs("dep_123").
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "desired_state", "desired_version", "synced_state", "synced_version", "attempt_count", "creator_is_member", "creator_assignment_pending", "name", "workos_org_id", "membership_id"}).
			AddRow("dep_123", DeploymentFGARegistered, 3, DeploymentFGARegistered, 2, 2, true, true, "Support agent", "org_123", "om_123"))

	work, err := NewDeploymentFGASyncStore(db, true).Pending(context.Background(), "dep_123")
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	want := DeploymentFGAWork{
		DeploymentID:             "dep_123",
		DesiredState:             DeploymentFGARegistered,
		DesiredVersion:           3,
		SyncedState:              DeploymentFGARegistered,
		SyncedVersion:            2,
		AttemptCount:             2,
		CreatorIsMember:          true,
		CreatorAssignmentPending: true,
		Name:                     "Support agent",
		WorkOSOrgID:              "org_123",
		MembershipID:             "om_123",
	}
	if work == nil || *work != want {
		t.Fatalf("Pending() = %#v, want %#v", work, want)
	}
}

func TestDeploymentFGASyncStoreRecordsOutcome(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	store := NewDeploymentFGASyncStore(db, true)

	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*synced_state`).
		WithArgs("dep_123", DeploymentFGADeleted, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	synced, err := store.MarkSynced(context.Background(), "dep_123", DeploymentFGADeleted, 2)
	if err != nil || !synced {
		t.Fatalf("MarkSynced() = %v, %v; want true, nil", synced, err)
	}

	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*creator_assignment_pending = TRUE`).
		WithArgs("dep_123", DeploymentFGARegistered, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deferred, err := store.DeferCreatorAssignment(context.Background(), "dep_123", DeploymentFGARegistered, 3)
	if err != nil || !deferred {
		t.Fatalf("DeferCreatorAssignment() = %v, %v; want true, nil", deferred, err)
	}

	cause := errors.New("WorkOS unavailable")
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("dep_456", DeploymentFGARegistered, cause.Error(), int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.RecordFailure(context.Background(), "dep_456", DeploymentFGARegistered, 4, cause); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}

	mock.ExpectQuery(`(?s)SELECT deployment_id.*OR creator_assignment_pending`).
		WithArgs(100).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow("dep_123"))
	ids, err := store.DueDeploymentIDs(context.Background(), 100)
	if err != nil || len(ids) != 1 || ids[0] != "dep_123" {
		t.Fatalf("DueDeploymentIDs() = %v, %v; want [dep_123], nil", ids, err)
	}

	mock.ExpectQuery(`(?s)SELECT EXISTS.*synced_version IS DISTINCT FROM s\.desired_version\)\s*\)`).
		WithArgs("acct_123").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	pending, err := store.HasPendingForAccount(context.Background(), "acct_123")
	if err != nil || !pending {
		t.Fatalf("HasPendingForAccount() = %v, %v; want true, nil", pending, err)
	}
}
