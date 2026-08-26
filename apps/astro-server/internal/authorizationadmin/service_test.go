package authorizationadmin

import (
	"context"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type fakeWorkOS struct {
	*authz.FakeFGA
	*authz.FakeGroups
	resources        []authz.AuthorizationResource
	deleted          []string
	listedOrgID      string
	listResourcesErr error
	listCalls        int
}

func (f *fakeWorkOS) ListAuthorizationResources(context.Context) ([]authz.AuthorizationResource, error) {
	f.listCalls++
	return f.resources, f.listResourcesErr
}

func (f *fakeWorkOS) ListAuthorizationResourcesForOrganization(_ context.Context, organizationID string) ([]authz.AuthorizationResource, error) {
	f.listedOrgID = organizationID
	return f.resources, f.listResourcesErr
}

func (f *fakeWorkOS) DeleteAuthorizationResource(_ context.Context, resourceID string) error {
	f.deleted = append(f.deleted, resourceID)
	return nil
}

type fakeOperationStore struct {
	operation       *Operation
	completedTarget int
	progressed      int
}

func (f *fakeOperationStore) Start(context.Context, string) (*Operation, error) {
	return f.operation, nil
}
func (f *fakeOperationStore) Progress(_ context.Context, _ string, _ int, processed, _ int, _ int, _ []ReportEntry) error {
	f.progressed = processed
	return nil
}
func (f *fakeOperationStore) Complete(_ context.Context, _ string, target, _ int, _ int, _ []ReportEntry) error {
	f.completedTarget = target
	return nil
}
func (*fakeOperationStore) Fail(context.Context, string, int, int, int, int, []ReportEntry, error) error {
	return nil
}
func TestInventoryUsesGenericResourcesAndKeepsDeploymentAccessSeparate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	resource := authz.DeploymentResource("dep_123")
	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
				return []authz.RoleAssignment{{
					Subject: authz.MembershipAssignmentSubject("om_admin"), Role: authz.RoleDeploymentAdmin,
					Source: authz.AssignmentSourceDirect, Resource: resource,
				}}, nil
			},
			ListGroupRoleAssignmentsFunc: func(_ context.Context, groupID string) ([]authz.RoleAssignment, error) {
				return []authz.RoleAssignment{{
					Subject: authz.GroupAssignmentSubject(groupID), Role: authz.RoleDeploymentBuilder,
					Source: authz.AssignmentSourceDirect, Resource: resource,
				}}, nil
			},
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{Groups: []authz.Group{{ID: "group_platform", Name: "Platform Engineering"}}}, nil
			},
		},
		resources: []authz.AuthorizationResource{
			{ID: "root", OrganizationID: "org_123", Resource: authz.ResourceRef{Type: authz.ResourceOrganization, ExternalID: "org_123"}},
			{ID: "workos_dep", OrganizationID: "org_123", Resource: resource, Name: "Support agent", CreatedAt: "2026-08-25T12:00:00Z"},
		},
	}
	mock.ExpectQuery(`SELECT mw.workos_membership_id`).WillReturnRows(sqlmock.NewRows([]string{"workos_membership_id", "label"}).
		AddRow("om_admin", "jessye@example.com"))
	mock.ExpectQuery(`SELECT d.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "account_name", "sync_state", "last_error"}).
		AddRow("dep_123", "acct_123", "Astro Spaceship", "synced", ""))
	store := &fakeOperationStore{}
	service := newService(db, workos, store)

	inventory, err := service.Inventory(context.Background())
	if err != nil || len(inventory.Resources) != 1 {
		t.Fatalf("Inventory() = (%+v, %v)", inventory, err)
	}
	got := inventory.Resources[0]
	if got.Type != "deployment" || got.AccountID != "acct_123" || got.AssignmentCount != 2 || !reflect.DeepEqual(got.DirectAdmins, []string{"jessye@example.com"}) {
		t.Fatalf("resource = %+v", got)
	}
	if got.Assignments[0].SubjectLabel != "Platform Engineering" || got.Assignments[0].Role != string(authz.RoleDeploymentBuilder) ||
		got.Assignments[1].SubjectLabel != "jessye@example.com" || got.Assignments[1].Role != string(authz.RoleDeploymentAdmin) {
		t.Fatalf("assignments = %+v", got.Assignments)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryMarksParentedDeploymentMissingFromDBAsWorkOSOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
				return nil, nil
			},
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		resources: []authz.AuthorizationResource{
			{ID: "workos_account", OrganizationID: "org_123", Resource: authz.ResourceRef{Type: authz.ResourceAccount, ExternalID: "acct_123"}, Name: "Astro Spaceship"},
			{ID: "workos_dep", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.DeploymentResource("dep_missing"), Name: "Missing deployment"},
		},
	}
	mock.ExpectQuery(`SELECT d.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "account_name", "sync_state", "last_error"}))
	service := newService(db, workos, &fakeOperationStore{})

	inventory, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if len(inventory.Resources) != 2 {
		t.Fatalf("resources = %+v", inventory.Resources)
	}
	deployment := inventory.Resources[1]
	if deployment.AccountID != "acct_123" || deployment.SyncState != "workos_only" {
		t.Fatalf("deployment = %+v", deployment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryCachesOnlyWorkOSSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
				return nil, nil
			},
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		resources: []authz.AuthorizationResource{{
			ID: "workos_dep", OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_123"),
		}},
	}
	rows := []string{"id", "account_id", "account_name", "sync_state", "last_error"}
	mock.ExpectQuery(`SELECT d.id`).WillReturnRows(sqlmock.NewRows(rows).AddRow("dep_123", "acct_123", "Astro Spaceship", "synced", ""))
	mock.ExpectQuery(`SELECT d.id`).WillReturnRows(sqlmock.NewRows(rows).AddRow("dep_123", "acct_123", "Astro Spaceship", "synced", ""))
	service := newService(db, workos, &fakeOperationStore{})

	if _, err := service.Inventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	if workos.listCalls != 1 {
		t.Fatalf("WorkOS list calls = %d, want 1", workos.listCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResetRemovesAssignmentsThenDeletesProductResources(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT COALESCE\(ao.workos_org_id, ''\)`).
		WithArgs("acct_123").
		WillReturnRows(sqlmock.NewRows([]string{"workos_org_id"}).AddRow("org_123"))

	confirmed := 1
	resource := authz.DeploymentResource("dep_123")
	var removed []authz.AssignmentSubject
	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
				return []authz.RoleAssignment{{Subject: authz.MembershipAssignmentSubject("om_admin"), Role: authz.RoleDeploymentAdmin, Source: authz.AssignmentSourceDirect, Resource: resource}}, nil
			},
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
			RemoveRoleFunc: func(_ context.Context, subject authz.AssignmentSubject, _ authz.RoleSlug, _ authz.ResourceRef) error {
				removed = append(removed, subject)
				return nil
			},
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		resources: []authz.AuthorizationResource{
			{ID: "root", OrganizationID: "org_123", Resource: authz.ResourceRef{Type: authz.ResourceOrganization, ExternalID: "org_123"}},
			{ID: "workos_dep", OrganizationID: "org_123", Resource: resource},
			{ID: "other_dep", OrganizationID: "org_456", Resource: authz.DeploymentResource("dep_other")},
		},
	}
	store := &fakeOperationStore{operation: &Operation{ID: "op_123", AccountID: "acct_123", AttemptCount: 1, ConfirmedCount: &confirmed}}
	service := newService(db, workos, store)

	if err := service.RunReset(context.Background(), "op_123"); err != nil {
		t.Fatalf("RunReset() error = %v", err)
	}
	if !reflect.DeepEqual(removed, []authz.AssignmentSubject{authz.MembershipAssignmentSubject("om_admin")}) {
		t.Fatalf("removed = %+v", removed)
	}
	if !reflect.DeepEqual(workos.deleted, []string{"workos_dep"}) {
		t.Fatalf("deleted = %v", workos.deleted)
	}
	if workos.listedOrgID != "org_123" {
		t.Fatalf("listed organization = %q", workos.listedOrgID)
	}
	if store.progressed != 1 || store.completedTarget != 1 {
		t.Fatalf("progress = %d, completed target = %d", store.progressed, store.completedTarget)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
