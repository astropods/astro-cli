package riverqueue

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

type deploymentFGATestQueue struct {
	reconciled []string
}

func (q *deploymentFGATestQueue) InsertLegacyDeploymentFGAReconcileJob(_ context.Context, deploymentID string) error {
	q.reconciled = append(q.reconciled, deploymentID)
	return nil
}

func (q *deploymentFGATestQueue) InsertDeploymentFGAReconcileJob(ctx context.Context, deploymentID string) error {
	return q.InsertLegacyDeploymentFGAReconcileJob(ctx, deploymentID)
}

type deploymentFGATestOrganizations struct {
	get func(context.Context, string) (org.Organization, error)
}

func (o *deploymentFGATestOrganizations) GetOrganization(ctx context.Context, organizationID string) (org.Organization, error) {
	return o.get(ctx, organizationID)
}

func TestDeploymentFGAReconcileRegistersResourceAndCreator(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:    "dep_123",
		DesiredState:    authz.DeploymentFGARegistered,
		DesiredVersion:  1,
		CreatorIsMember: true,
		Name:            "Support agent",
		WorkOSOrgID:     "org_123",
		MembershipID:    "om_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	var registered, assigned bool
	fga := &authz.FakeFGA{
		RegisterResourceFunc: func(_ context.Context, organizationID string, resource authz.ResourceRef, name string) error {
			registered = organizationID == "org_123" && resource == authz.DeploymentResource("dep_123") && name == "Support agent"
			return nil
		},
		AssignRoleFunc: func(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
			assigned = subject == authz.MembershipAssignmentSubject("om_123") && role == authz.RoleDeploymentAdmin && resource == authz.DeploymentResource("dep_123")
			return nil
		},
	}

	worker := deploymentFGAWorker(db, fga)
	if err := worker.Work(context.Background(), &river.Job[DeploymentFGAReconcileArgs]{Args: DeploymentFGAReconcileArgs{DeploymentID: "dep_123"}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if !registered || !assigned {
		t.Fatalf("registered = %v, assigned = %v; want both true", registered, assigned)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentFGAReconcileRegistersWithoutCreatorMembership(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGARegistered,
		DesiredVersion: 1,
		Name:           "Support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	var registered bool
	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		RegisterResourceFunc: func(_ context.Context, organizationID string, resource authz.ResourceRef, name string) error {
			registered = organizationID == "org_123" && resource == authz.DeploymentResource("dep_123") && name == "Support agent"
			return nil
		},
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if !registered {
		t.Fatal("RegisterResource() was not called")
	}
}

func TestDeploymentFGAReconcileRetriesMissingCreatorMembership(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:    "dep_123",
		DesiredState:    authz.DeploymentFGARegistered,
		DesiredVersion:  1,
		CreatorIsMember: true,
		Name:            "Support agent",
		WorkOSOrgID:     "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("dep_123", authz.DeploymentFGARegistered, errDeploymentCreatorMembershipUnavailable.Error(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var registered bool
	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		RegisterResourceFunc: func(context.Context, string, authz.ResourceRef, string) error {
			registered = true
			return nil
		},
	})
	err := worker.reconcile(context.Background(), "dep_123")
	if !errors.Is(err, errDeploymentCreatorMembershipUnavailable) {
		t.Fatalf("reconcile() error = %v, want %v", err, errDeploymentCreatorMembershipUnavailable)
	}
	if !registered {
		t.Fatal("RegisterResource() was not called before retrying membership resolution")
	}
}

func TestDeploymentFGAReconcileDefersCreatorAssignmentAfterRetryLimit(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:    "dep_123",
		DesiredState:    authz.DeploymentFGARegistered,
		DesiredVersion:  1,
		AttemptCount:    deploymentFGAMembershipRetryLimit,
		CreatorIsMember: true,
		Name:            "Support agent",
		WorkOSOrgID:     "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*creator_assignment_pending = TRUE`).
		WithArgs("dep_123", authz.DeploymentFGARegistered, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		RegisterResourceFunc: func(context.Context, string, authz.ResourceRef, string) error { return nil },
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileAssignsCreatorAfterDeferredMembershipResolves(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:             "dep_123",
		DesiredState:             authz.DeploymentFGARegistered,
		DesiredVersion:           1,
		SyncedState:              authz.DeploymentFGARegistered,
		SyncedVersion:            1,
		CreatorIsMember:          true,
		CreatorAssignmentPending: true,
		Name:                     "Support agent",
		WorkOSOrgID:              "org_123",
		MembershipID:             "om_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	var assigned bool
	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		AssignRoleFunc: func(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
			assigned = subject == authz.MembershipAssignmentSubject("om_123") && role == authz.RoleDeploymentAdmin && resource == authz.DeploymentResource("dep_123")
			return nil
		},
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if !assigned {
		t.Fatal("AssignRole() was not called after membership resolution")
	}
}

func TestDeploymentFGAReconcileClearsDeferredAssignmentForDepartedCreator(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:             "dep_123",
		DesiredState:             authz.DeploymentFGARegistered,
		DesiredVersion:           1,
		SyncedState:              authz.DeploymentFGARegistered,
		SyncedVersion:            1,
		CreatorAssignmentPending: true,
		Name:                     "Support agent",
		WorkOSOrgID:              "org_123",
		MembershipID:             "om_stale",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	worker := deploymentFGAWorker(db, &authz.FakeFGA{})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileDisabledConvergesPendingWork(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGARegistered,
		DesiredVersion: 1,
		Name:           "Support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)
	worker := deploymentFGAWorker(db, nil)
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	pending, err := authz.NewDeploymentFGASyncStore(db, false).HasPendingForAccount(context.Background(), "acct_123")
	if err != nil {
		t.Fatalf("HasPendingForAccount() error = %v", err)
	}
	if pending {
		t.Fatal("HasPendingForAccount() = true, want false with WorkOS disabled")
	}
}

func TestDeploymentFGAReconcileTreatsIdempotentResultsAsSuccess(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:    "dep_123",
		DesiredState:    authz.DeploymentFGARegistered,
		DesiredVersion:  2,
		CreatorIsMember: true,
		Name:            "Support agent",
		WorkOSOrgID:     "org_123",
		MembershipID:    "om_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		RegisterResourceFunc: func(context.Context, string, authz.ResourceRef, string) error {
			return authz.ErrResourceExists
		},
		UpdateResourceNameFunc: func(context.Context, string, authz.ResourceRef, string) error {
			return nil
		},
		AssignRoleFunc: func(context.Context, authz.AssignmentSubject, authz.RoleSlug, authz.ResourceRef) error {
			return authz.ErrRoleAssignmentExists
		},
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileUpdatesRegisteredResourceName(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGARegistered,
		DesiredVersion: 2,
		SyncedState:    authz.DeploymentFGARegistered,
		SyncedVersion:  1,
		Name:           "Renamed support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	var updated bool
	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		UpdateResourceNameFunc: func(_ context.Context, organizationID string, resource authz.ResourceRef, name string) error {
			updated = organizationID == "org_123" && resource == authz.DeploymentResource("dep_123") && name == "Renamed support agent"
			return nil
		},
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if !updated {
		t.Fatal("UpdateResourceName() was not called with the renamed resource")
	}
}

func TestDeploymentFGAReconcileDeleteTreatsMissingAsSuccess(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGADeleted,
		DesiredVersion: 2,
		Name:           "Support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		DeleteResourceFunc: func(context.Context, string, authz.ResourceRef) error {
			return authz.ErrResourceNotFound
		},
	})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileDeleteConvergesWhenOrganizationIsGone(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGADeleted,
		DesiredVersion: 2,
		Name:           "Support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	deleteErr := errors.New("WorkOS resource delete failed")
	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		DeleteResourceFunc: func(context.Context, string, authz.ResourceRef) error {
			return deleteErr
		},
	})
	worker.organizations = &deploymentFGATestOrganizations{
		get: func(_ context.Context, organizationID string) (org.Organization, error) {
			if organizationID != "org_123" {
				t.Fatalf("organization id = %q, want org_123", organizationID)
			}
			return org.Organization{}, org.ErrOrganizationNotFound
		},
	}
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileDeleteConvergesWithoutOrganizationID(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGADeleted,
		DesiredVersion: 2,
		Name:           "Support agent",
	}
	expectDeploymentFGAWork(mock, work)
	expectDeploymentFGAMarkSynced(mock, work)

	worker := deploymentFGAWorker(db, &authz.FakeFGA{})
	if err := worker.reconcile(context.Background(), "dep_123"); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
}

func TestDeploymentFGAReconcileDeleteRetriesWhileOrganizationExists(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:   "dep_123",
		DesiredState:   authz.DeploymentFGADeleted,
		DesiredVersion: 2,
		Name:           "Support agent",
		WorkOSOrgID:    "org_123",
	}
	expectDeploymentFGAWork(mock, work)
	deleteErr := errors.New("WorkOS resource delete failed")
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("dep_123", authz.DeploymentFGADeleted, deleteErr.Error(), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		DeleteResourceFunc: func(context.Context, string, authz.ResourceRef) error {
			return deleteErr
		},
	})
	worker.organizations = &deploymentFGATestOrganizations{
		get: func(context.Context, string) (org.Organization, error) {
			return org.Organization{ID: "org_123"}, nil
		},
	}
	if err := worker.reconcile(context.Background(), "dep_123"); !errors.Is(err, deleteErr) {
		t.Fatalf("reconcile() error = %v, want %v", err, deleteErr)
	}
}

func TestDeploymentFGAReconcileRecordsFailure(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	work := authz.DeploymentFGAWork{
		DeploymentID:    "dep_123",
		DesiredState:    authz.DeploymentFGARegistered,
		DesiredVersion:  1,
		CreatorIsMember: true,
		Name:            "Support agent",
		WorkOSOrgID:     "org_123",
		MembershipID:    "om_123",
	}
	expectDeploymentFGAWork(mock, work)
	cause := errors.New("WorkOS unavailable")
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("dep_123", authz.DeploymentFGARegistered, cause.Error(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	worker := deploymentFGAWorker(db, &authz.FakeFGA{
		RegisterResourceFunc: func(context.Context, string, authz.ResourceRef, string) error {
			return cause
		},
	})
	err := worker.reconcile(context.Background(), "dep_123")
	if !errors.Is(err, cause) {
		t.Fatalf("reconcile() error = %v, want %v", err, cause)
	}
}

func TestDeploymentFGAReconcileSweepEnqueuesDueWork(t *testing.T) {
	db, mock := deploymentFGATestDB(t)
	mock.ExpectQuery(`SELECT deployment_id`).
		WithArgs(deploymentFGASweepLimit).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow("dep_123"))

	queue := &deploymentFGATestQueue{}
	worker := deploymentFGAWorker(db, &authz.FakeFGA{})
	worker.queue = queue
	if err := worker.Work(context.Background(), &river.Job[DeploymentFGAReconcileArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.reconciled) != 1 || queue.reconciled[0] != "dep_123" {
		t.Fatalf("reconciled = %v, want [dep_123]", queue.reconciled)
	}
}

func deploymentFGATestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return db, mock
}

func deploymentFGAWorker(db *sql.DB, fga authz.FGA) *DeploymentFGAReconcileWorker {
	return &DeploymentFGAReconcileWorker{
		fga:  fga,
		sync: authz.NewDeploymentFGASyncStore(db, fga != nil),
		log:  logger.New("error", "json"),
	}
}

func expectDeploymentFGAWork(mock sqlmock.Sqlmock, work authz.DeploymentFGAWork) {
	mock.ExpectQuery(`(?s)SELECT s\.deployment_id.*OR s\.creator_assignment_pending`).
		WithArgs(work.DeploymentID).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "desired_state", "desired_version", "synced_state", "synced_version", "attempt_count", "creator_is_member", "creator_assignment_pending", "name", "workos_org_id", "membership_id"}).
			AddRow(work.DeploymentID, work.DesiredState, work.DesiredVersion, work.SyncedState, work.SyncedVersion, work.AttemptCount, work.CreatorIsMember, work.CreatorAssignmentPending, work.Name, work.WorkOSOrgID, work.MembershipID))
}

func expectDeploymentFGAMarkSynced(mock sqlmock.Sqlmock, work authz.DeploymentFGAWork) {
	mock.ExpectExec(`(?s)UPDATE deployment_fga_sync.*synced_state`).
		WithArgs(work.DeploymentID, work.DesiredState, work.DesiredVersion).
		WillReturnResult(sqlmock.NewResult(0, 1))
}
