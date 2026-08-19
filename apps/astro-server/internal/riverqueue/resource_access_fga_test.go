package riverqueue

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type resourceAccessFGATestQueue struct {
	keys []authz.AccessIntentKey
}

func (q *resourceAccessFGATestQueue) InsertResourceAccessFGAReconcileJob(_ context.Context, key authz.AccessIntentKey) error {
	q.keys = append(q.keys, key)
	return nil
}

func TestResourceAccessFGAReconcileBatchesResourceIntents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`(?s)FROM resource_access_fga_sync.*synced_version IS DISTINCT FROM desired_version`).
		WithArgs("org_123", authz.ResourceDeployment, "dep_123").
		WillReturnRows(sqlmock.NewRows(accessIntentColumnsForWorker()).AddRow(
			"acct_123", "org_123", authz.ResourceDeployment, "dep_123",
			authz.AssignmentSubjectMembership, "user_123", "om_123", authz.RoleDeploymentBuilder,
			int64(1), nil, nil, 0, nil, now, nil, now,
		).AddRow(
			"acct_123", "org_123", authz.ResourceDeployment, "dep_123",
			authz.AssignmentSubjectMembership, "user_456", "om_456", authz.RoleDeploymentViewer,
			int64(1), nil, nil, 0, nil, now, nil, now,
		))
	mock.ExpectExec(`(?s)UPDATE resource_access_fga_sync.*synced_version = \$7`).
		WithArgs("org_123", authz.ResourceDeployment, "dep_123", authz.AssignmentSubjectMembership, "om_123", authz.RoleDeploymentBuilder, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE resource_access_fga_sync.*synced_version = \$7`).
		WithArgs("org_123", authz.ResourceDeployment, "dep_123", authz.AssignmentSubjectMembership, "om_456", authz.RoleDeploymentViewer, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	listCalls := 0
	assigned := make(map[string]authz.RoleSlug)
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
			listCalls++
			return nil, nil
		},
		AssignRoleFunc: func(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
			if resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("AssignRole() resource = %+v", resource)
			}
			assigned[subject.ID] = role
			return nil
		},
	}
	store := authz.NewResourceAccessSyncStore(db)
	worker := &ResourceAccessFGAReconcileWorker{store: store, reconciler: authz.NewAccessReconciler(fga, store)}
	job := &river.Job[ResourceAccessFGAReconcileArgs]{Args: ResourceAccessFGAReconcileArgs{
		OrganizationID: "org_123", ResourceType: "deployment", ResourceID: "dep_123",
	}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if listCalls != 1 || assigned["om_123"] != authz.RoleDeploymentBuilder || assigned["om_456"] != authz.RoleDeploymentViewer {
		t.Fatalf("ListRoleAssignments() calls=%d assigned=%v", listCalls, assigned)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceAccessFGASweepEnqueuesDueIntents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`(?s)SELECT organization_id, resource_type, resource_id, subject_type, workos_subject_id.*FROM resource_access_fga_sync`).
		WithArgs(resourceAccessFGASweepLimit).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "resource_type", "resource_id", "subject_type", "workos_subject_id"}).
			AddRow("org_123", "deployment", "dep_123", "organization_membership", "om_123").
			AddRow("org_123", "deployment", "dep_123", "organization_membership", "om_456"))
	queue := &resourceAccessFGATestQueue{}
	store := authz.NewResourceAccessSyncStore(db)
	worker := &ResourceAccessFGAReconcileWorker{store: store, reconciler: authz.NewAccessReconciler(&authz.FakeFGA{}, store), queue: queue}
	if err := worker.Work(context.Background(), &river.Job[ResourceAccessFGAReconcileArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.keys) != 1 || queue.keys[0].Resource != authz.DeploymentResource("dep_123") || queue.keys[0].Subject.ID != "" {
		t.Fatalf("queued keys = %+v", queue.keys)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func accessIntentColumnsForWorker() []string {
	return []string{
		"account_id", "organization_id", "resource_type", "resource_id",
		"subject_type", "subject_id", "workos_subject_id", "desired_role",
		"desired_version", "synced_role", "synced_version", "attempt_count",
		"last_error", "next_attempt_at", "synced_at", "updated_at",
	}
}
