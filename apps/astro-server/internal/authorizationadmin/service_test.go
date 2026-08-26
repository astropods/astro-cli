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
	listResourcesErr error
	listCalls        int
}

func (f *fakeWorkOS) ListAuthorizationResources(context.Context) ([]authz.AuthorizationResource, error) {
	f.listCalls++
	return f.resources, f.listResourcesErr
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
	service := NewService(db, workos)

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
	service := NewService(db, workos)

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
	service := NewService(db, workos)

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
