package authz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var accessIntentColumns = []string{
	"account_id", "organization_id", "resource_type", "resource_id",
	"subject_type", "subject_id", "workos_subject_id", "desired_role",
	"desired_version", "synced_role", "synced_version", "attempt_count",
	"last_error", "next_attempt_at", "synced_at", "updated_at",
}

func TestResourceAccessSyncStoreRecordsDesiredRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	columns := append(append([]string(nil), accessIntentColumns...), "changed")
	mock.ExpectQuery(`(?s)INSERT INTO resource_access_fga_sync.*ON CONFLICT.*desired_role IS DISTINCT`).
		WithArgs("acct_123", "org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentBuilder).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			"acct_123", "org_123", ResourceDeployment, "dep_123",
			AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentBuilder,
			int64(2), RoleDeploymentViewer, int64(1), 0, nil, now, nil, now, true,
		))

	intent, changed, err := NewResourceAccessSyncStore(db).Record(context.Background(), AccessIntent{
		AccountID: "acct_123", OrganizationID: "org_123", Resource: DeploymentResource("dep_123"),
		Subject: MembershipAssignmentSubject("om_123"), SubjectID: "user_123", DesiredRole: RoleDeploymentBuilder,
	})
	if err != nil || !changed || intent.Status() != AccessSyncPending || intent.DesiredVersion != 2 {
		t.Fatalf("Record() intent=%+v changed=%t error=%v", intent, changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAccessSyncStoreRetriesConcurrentNoOpRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	columns := append(append([]string(nil), accessIntentColumns...), "changed")
	mock.ExpectQuery(`(?s)INSERT INTO resource_access_fga_sync.*ON CONFLICT`).
		WithArgs("acct_123", "org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentViewer).
		WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectQuery(`(?s)INSERT INTO resource_access_fga_sync.*ON CONFLICT`).
		WithArgs("acct_123", "org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentViewer).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			"acct_123", "org_123", ResourceDeployment, "dep_123",
			AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentViewer,
			int64(1), RoleDeploymentViewer, int64(1), 0, nil, now, now, now, false,
		))

	intent, changed, err := NewResourceAccessSyncStore(db).Record(context.Background(), AccessIntent{
		AccountID: "acct_123", OrganizationID: "org_123", Resource: DeploymentResource("dep_123"),
		Subject: MembershipAssignmentSubject("om_123"), SubjectID: "user_123", DesiredRole: RoleDeploymentViewer,
	})
	if err != nil || changed || intent.Status() != AccessSyncSynced {
		t.Fatalf("Record() intent=%+v changed=%t error=%v", intent, changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAccessSyncStoreLoadsPendingAndRecordsOutcome(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`(?s)FROM resource_access_fga_sync.*synced_version IS DISTINCT FROM desired_version`).
		WithArgs("org_123", ResourceDeployment, "dep_123").
		WillReturnRows(sqlmock.NewRows(accessIntentColumns).AddRow(
			"acct_123", "org_123", ResourceDeployment, "dep_123",
			AssignmentSubjectMembership, "user_123", "om_123", RoleDeploymentBuilder,
			int64(3), RoleDeploymentViewer, int64(2), 1, "temporary failure", now, nil, now,
		))
	store := NewResourceAccessSyncStore(db)
	intents, err := store.PendingForResource(context.Background(), "org_123", DeploymentResource("dep_123"))
	if err != nil || len(intents) != 1 || intents[0].Status() != AccessSyncRetrying {
		t.Fatalf("PendingForResource() intents=%+v error=%v", intents, err)
	}
	intent := intents[0]

	cause := errors.New("WorkOS unavailable")
	mock.ExpectExec(`(?s)UPDATE resource_access_fga_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "om_123", cause.Error(), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.RecordFailure(context.Background(), intent, cause); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}

	mock.ExpectExec(`(?s)UPDATE resource_access_fga_sync.*synced_version = \$7`).
		WithArgs("org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "om_123", RoleDeploymentBuilder, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	synced, err := store.MarkSynced(context.Background(), intent)
	if err != nil || !synced {
		t.Fatalf("MarkSynced() = %t, %v", synced, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAccessSyncStoreDiscardsMatchingVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	intent := AccessIntent{
		OrganizationID: "org_123", Resource: DeploymentResource("dep_123"),
		Subject: MembershipAssignmentSubject("om_123"), DesiredVersion: 3,
	}
	mock.ExpectExec(`(?s)DELETE FROM resource_access_fga_sync.*desired_version = \$6`).
		WithArgs("org_123", ResourceDeployment, "dep_123", AssignmentSubjectMembership, "om_123", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	discarded, err := NewResourceAccessSyncStore(db).Discard(context.Background(), intent)
	if err != nil || !discarded {
		t.Fatalf("Discard() = %t, %v", discarded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccessIntentStatus(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		intent AccessIntent
		want   AccessSyncStatus
	}{
		{intent: AccessIntent{DesiredVersion: 1}, want: AccessSyncPending},
		{intent: AccessIntent{DesiredVersion: 2, SyncedVersion: 1, LastError: "failed"}, want: AccessSyncRetrying},
		{intent: AccessIntent{DesiredVersion: 2, SyncedVersion: 2, LastError: "old"}, want: AccessSyncSynced},
	} {
		if got := test.intent.Status(); got != test.want {
			t.Fatalf("Status() = %q, want %q", got, test.want)
		}
	}
}
