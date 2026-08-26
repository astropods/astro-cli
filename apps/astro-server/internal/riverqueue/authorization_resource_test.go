package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type authorizationLifecycleFake struct {
	registered []authz.ResourceRef
	parents    []authz.ResourceRef
	assigned   []authz.RoleSlug
}

var authorizationResourcePendingColumns = []string{
	"account_id", "organization_id", "resource_type", "resource_id", "parent_type", "parent_id",
	"name", "desired_state", "desired_version", "synced_state", "synced_version", "workos_id",
	"creator_user_id", "creator_role", "creator_pending", "attempt_count", "creator_is_member", "membership_id",
}

func (f *authorizationLifecycleFake) RegisterResourceWithParent(_ context.Context, _ string, resource, parent authz.ResourceRef, _ string) (string, error) {
	f.registered = append(f.registered, resource)
	f.parents = append(f.parents, parent)
	return "workos-" + resource.ExternalID, nil
}

func (*authorizationLifecycleFake) UpdateResourceName(context.Context, string, authz.ResourceRef, string) error {
	return nil
}

func (*authorizationLifecycleFake) DeleteResource(context.Context, string, authz.ResourceRef) error {
	return nil
}

func (f *authorizationLifecycleFake) AssignRole(_ context.Context, _ authz.AssignmentSubject, role authz.RoleSlug, _ authz.ResourceRef) error {
	f.assigned = append(f.assigned, role)
	return nil
}

func TestAuthorizationResourceReconcileRegistersParentBeforeBlueprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT s.account_id`).
		WithArgs("org-1", authz.ResourceBlueprint, "blueprint-1").
		WillReturnRows(sqlmock.NewRows(authorizationResourcePendingColumns).
			AddRow("acct-1", "org-1", "blueprint", "blueprint-1", "account", "acct-1", "Coach", "registered", 1, "", 0, "", "user-1", "blueprint-admin", false, 0, true, "om-user-1"))
	mock.ExpectQuery(`SELECT s.account_id`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1").
		WillReturnRows(sqlmock.NewRows(authorizationResourcePendingColumns).
			AddRow("acct-1", "org-1", "account", "acct-1", "organization", "org-1", "Astro Spaceship", "registered", 1, "", 0, "", "owner-1", "account-admin", false, 0, true, "om-owner-1"))
	mock.ExpectExec(`UPDATE authorization_resource_sync`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1", authz.AuthorizationResourceRegistered, int64(1), "workos-acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE authorization_resource_sync`).
		WithArgs("org-1", authz.ResourceBlueprint, "blueprint-1", authz.AuthorizationResourceRegistered, int64(1), "workos-blueprint-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	lifecycle := &authorizationLifecycleFake{}
	worker := &AuthorizationResourceReconcileWorker{
		lifecycle: lifecycle,
		sync:      authz.NewAuthorizationResourceSyncStore(db, true),
		log:       logger.New("error", "json"),
	}
	err = worker.Work(context.Background(), &river.Job[AuthorizationResourceReconcileArgs]{Args: AuthorizationResourceReconcileArgs{
		OrganizationID: "org-1", ResourceType: "blueprint", ResourceID: "blueprint-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle.registered) != 2 || lifecycle.registered[0] != authz.AccountResource("acct-1") || lifecycle.registered[1] != authz.BlueprintResource("blueprint-1") {
		t.Fatalf("registration order = %+v", lifecycle.registered)
	}
	if lifecycle.parents[0].Type != authz.ResourceOrganization || lifecycle.parents[1] != authz.AccountResource("acct-1") {
		t.Fatalf("parents = %+v", lifecycle.parents)
	}
	if len(lifecycle.assigned) != 2 || lifecycle.assigned[0] != authz.RoleAccountAdmin || lifecycle.assigned[1] != authz.RoleBlueprintAdmin {
		t.Fatalf("roles = %+v", lifecycle.assigned)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationResourceReconcilePersistsRegistrationBeforeCreatorRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	key := authz.ResourceSyncKey{OrganizationID: "org-1", Resource: authz.AccountResource("acct-1")}
	mock.ExpectQuery(`SELECT s.account_id`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1").
		WillReturnRows(sqlmock.NewRows(authorizationResourcePendingColumns).
			AddRow("acct-1", "org-1", "account", "acct-1", "organization", "org-1", "Astro Spaceship", "registered", 1, "", 0, "", "owner-1", "account-admin", false, 0, true, ""))
	mock.ExpectExec(`(?s)UPDATE authorization_resource_sync.*creator_assignment_pending = TRUE`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1", authz.AuthorizationResourceRegistered, int64(1), "workos-acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE authorization_resource_sync.*attempt_count = attempt_count \+ 1`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1", errAuthorizationCreatorMembershipUnavailable.Error(), authz.AuthorizationResourceRegistered, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT s.account_id`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1").
		WillReturnRows(sqlmock.NewRows(authorizationResourcePendingColumns).
			AddRow("acct-1", "org-1", "account", "acct-1", "organization", "org-1", "Astro Spaceship", "registered", 1, "registered", 1, "workos-acct-1", "owner-1", "account-admin", true, 1, true, "om-owner-1"))
	mock.ExpectExec(`(?s)UPDATE authorization_resource_sync.*creator_assignment_pending = FALSE`).
		WithArgs("org-1", authz.ResourceAccount, "acct-1", authz.AuthorizationResourceRegistered, int64(1), "workos-acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	lifecycle := &authorizationLifecycleFake{}
	worker := &AuthorizationResourceReconcileWorker{
		lifecycle: lifecycle,
		sync:      authz.NewAuthorizationResourceSyncStore(db, true),
		log:       logger.New("error", "json"),
	}
	job := &river.Job[AuthorizationResourceReconcileArgs]{Args: AuthorizationResourceReconcileArgs{
		OrganizationID: key.OrganizationID,
		ResourceType:   string(key.Resource.Type),
		ResourceID:     key.Resource.ExternalID,
	}}
	if err := worker.Work(context.Background(), job); !errors.Is(err, errAuthorizationCreatorMembershipUnavailable) {
		t.Fatalf("first Work() error = %v", err)
	}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}
	if len(lifecycle.registered) != 1 {
		t.Fatalf("registrations = %d, want 1", len(lifecycle.registered))
	}
	if len(lifecycle.assigned) != 1 || lifecycle.assigned[0] != authz.RoleAccountAdmin {
		t.Fatalf("assigned roles = %+v", lifecycle.assigned)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
