package authorizationadmin

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/authzbackfill"
)

type fakeWorkOS struct {
	*authz.FakeFGA
	*authz.FakeGroups
	mu               sync.Mutex
	resources        []authz.AuthorizationResource
	deleted          []string
	listedOrgID      string
	listedOrgIDs     []string
	listResourcesErr error
	listCalls        int
}

func (f *fakeWorkOS) ListAuthorizationResourcesForOrganization(_ context.Context, organizationID string) ([]authz.AuthorizationResource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	f.listedOrgID = organizationID
	f.listedOrgIDs = append(f.listedOrgIDs, organizationID)
	resources := make([]authz.AuthorizationResource, 0)
	for _, resource := range f.resources {
		if resource.OrganizationID == organizationID {
			resources = append(resources, resource)
		}
	}
	return resources, f.listResourcesErr
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
	f.operation.AttemptCount++
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

func expectLinkedOrganizations(mock sqlmock.Sqlmock, organizationIDs ...string) {
	rows := sqlmock.NewRows([]string{"workos_org_id"})
	for _, organizationID := range organizationIDs {
		rows.AddRow(organizationID)
	}
	mock.ExpectQuery(`SELECT DISTINCT ao.workos_org_id`).WillReturnRows(rows)
}

func expectLocalAuthorizationResources(
	mock sqlmock.Sqlmock,
	accounts []authzbackfill.Account,
	resources []authzbackfill.Resource,
) {
	accountRows := sqlmock.NewRows([]string{"id", "organization_id", "name", "owner_membership_id"})
	for _, account := range accounts {
		accountRows.AddRow(account.ID, account.OrganizationID, account.Name, account.OwnerMembershipID)
	}
	mock.ExpectQuery(`SELECT a.id::text`).WillReturnRows(accountRows)
	if len(accounts) != 0 {
		resourceRows := sqlmock.NewRows([]string{"account_id", "resource_type", "external_id", "name"})
		for _, resource := range resources {
			resourceRows.AddRow(resource.AccountID, resource.Ref.Type, resource.Ref.ExternalID, resource.Name)
		}
		mock.ExpectQuery(`SELECT account_id::text, resource_type, external_id, name`).WillReturnRows(resourceRows)
		mock.ExpectQuery(`SELECT a.id::text`).WillReturnRows(
			sqlmock.NewRows([]string{"id", "organization_id", "name", "owner_membership_id"}),
		)
	}
}

func TestRunBackfillCompletesDryRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT count\(\*\) FROM agents WHERE uid IS NULL`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT a.id::text`).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "owner_membership_id"}))
	store := &fakeOperationStore{operation: &Operation{ID: "op_123", Kind: "resource_backfill", DryRun: true}}
	service := newService(db, &fakeWorkOS{FakeFGA: &authz.FakeFGA{}, FakeGroups: &authz.FakeGroups{}}, store)

	if err := service.RunBackfill(context.Background(), "op_123"); err != nil {
		t.Fatalf("RunBackfill() error = %v", err)
	}
	if store.completedTarget != 2 {
		t.Fatalf("completed target = %d, want 2 missing Blueprint resources", store.completedTarget)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
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
			ListRoleAssignmentsFunc: func(_ context.Context, _ string, current authz.ResourceRef) ([]authz.RoleAssignment, error) {
				if current != resource {
					return nil, nil
				}
				return []authz.RoleAssignment{{
					Subject: authz.MembershipAssignmentSubject("om_admin"), Role: authz.RoleDeploymentAdmin,
					Source: authz.AssignmentSourceDirect, Resource: resource,
				}}, nil
			},
			ListGroupRoleAssignmentsFunc: func(_ context.Context, groupID string) ([]authz.RoleAssignment, error) {
				return []authz.RoleAssignment{{
					Subject: authz.GroupAssignmentSubject(groupID), Role: authz.RoleDeploymentMaintainer,
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
			{ID: "workos_account", OrganizationID: "org_123", Resource: authz.AccountResource("acct_123"), Name: "Astro Spaceship"},
			{ID: "workos_dep", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: resource, Name: "Support agent", CreatedAt: "2026-08-25T12:00:00Z"},
		},
	}
	expectLinkedOrganizations(mock, "org_123")
	mock.ExpectQuery(`SELECT mw.workos_membership_id`).WillReturnRows(sqlmock.NewRows([]string{"workos_membership_id", "label"}).
		AddRow("om_admin", "jessye@example.com"))
	expectLocalAuthorizationResources(mock,
		[]authzbackfill.Account{{ID: "acct_123", OrganizationID: "org_123", Name: "Astro Spaceship"}},
		[]authzbackfill.Resource{{AccountID: "acct_123", Ref: resource, Name: "Support agent"}},
	)
	store := &fakeOperationStore{}
	service := newService(db, workos, store)

	inventory, err := service.Inventory(context.Background())
	if err != nil || len(inventory.Resources) != 2 {
		t.Fatalf("Inventory() = (%+v, %v)", inventory, err)
	}
	got := inventory.Resources[1]
	if got.Type != "deployment" || got.AccountID != "acct_123" || got.AssignmentCount != 2 || !reflect.DeepEqual(got.DirectAdmins, []string{"jessye@example.com"}) {
		t.Fatalf("resource = %+v", got)
	}
	if got.Assignments[0].SubjectLabel != "Platform Engineering" || got.Assignments[0].Role != string(authz.RoleDeploymentMaintainer) ||
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
			{ID: "workos_insights", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.InsightsResource("acct_123"), Name: "Insights"},
			{ID: "workos_blueprint", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.BlueprintResource("blueprint_123"), Name: "Support blueprint"},
			{ID: "workos_dep", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.DeploymentResource("dep_missing"), Name: "Missing deployment"},
		},
	}
	expectLinkedOrganizations(mock, "org_123")
	expectLocalAuthorizationResources(mock,
		[]authzbackfill.Account{{ID: "acct_123", OrganizationID: "org_123", Name: "Astro Spaceship"}},
		[]authzbackfill.Resource{
			{AccountID: "acct_123", Ref: authz.InsightsResource("acct_123"), Name: "Insights"},
			{AccountID: "acct_123", Ref: authz.BlueprintResource("blueprint_123"), Name: "Support blueprint"},
		},
	)
	service := newService(db, workos, &fakeOperationStore{})

	inventory, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if len(inventory.Resources) != 4 {
		t.Fatalf("resources = %+v", inventory.Resources)
	}
	if blueprint := inventory.Resources[1]; blueprint.Type != "blueprint" || blueprint.SyncState != "registered" {
		t.Fatalf("blueprint = %+v", blueprint)
	}
	deployment := inventory.Resources[2]
	if deployment.AccountID != "acct_123" || deployment.SyncState != "workos_only" {
		t.Fatalf("deployment = %+v", deployment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryMarksActiveAstroResourceMissingFromWorkOS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc:      func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) { return nil, nil },
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		resources: []authz.AuthorizationResource{
			{ID: "workos_account", OrganizationID: "org_123", Resource: authz.AccountResource("acct_123"), Name: "Astro Spaceship"},
			{ID: "workos_insights", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.InsightsResource("acct_123"), Name: "Insights"},
		},
	}
	expectLinkedOrganizations(mock, "org_123")
	expectLocalAuthorizationResources(mock,
		[]authzbackfill.Account{{ID: "acct_123", OrganizationID: "org_123", Name: "Astro Spaceship"}},
		[]authzbackfill.Resource{
			{AccountID: "acct_123", Ref: authz.InsightsResource("acct_123"), Name: "Insights"},
			{AccountID: "acct_123", Ref: authz.DeploymentResource("dep_missing"), Name: "Missing deployment"},
		},
	)

	inventory, err := newService(db, workos, &fakeOperationStore{}).Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	for _, resource := range inventory.Resources {
		if resource.ExternalID == "dep_missing" {
			if resource.SyncState != "missing_in_workos" || resource.WorkOSResourceID != "" || resource.AccountID != "acct_123" {
				t.Fatalf("missing resource = %+v", resource)
			}
			return
		}
	}
	t.Fatalf("missing resource not found in inventory: %+v", inventory.Resources)
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
		resources: []authz.AuthorizationResource{
			{ID: "workos_account", OrganizationID: "org_123", Resource: authz.AccountResource("acct_123")},
			{ID: "workos_insights", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.InsightsResource("acct_123")},
			{ID: "workos_dep", OrganizationID: "org_123", ParentResourceID: "workos_account", Resource: authz.DeploymentResource("dep_123")},
		},
	}
	expectLinkedOrganizations(mock, "org_123")
	accounts := []authzbackfill.Account{{ID: "acct_123", OrganizationID: "org_123", Name: "Astro Spaceship"}}
	resources := []authzbackfill.Resource{
		{AccountID: "acct_123", Ref: authz.InsightsResource("acct_123"), Name: "Insights"},
		{AccountID: "acct_123", Ref: authz.DeploymentResource("dep_123"), Name: "Support agent"},
	}
	expectLocalAuthorizationResources(mock, accounts, resources)
	expectLocalAuthorizationResources(mock, accounts, resources)
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

func TestInventoryListsResourcesByLinkedOrganization(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc:      func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) { return nil, nil },
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		resources: []authz.AuthorizationResource{
			{ID: "account_a", OrganizationID: "org_a", Resource: authz.ResourceRef{Type: authz.ResourceAccount, ExternalID: "acct_a"}},
			{ID: "account_b", OrganizationID: "org_b", Resource: authz.ResourceRef{Type: authz.ResourceAccount, ExternalID: "acct_b"}},
		},
	}
	expectLinkedOrganizations(mock, "org_a", "org_b")
	expectLocalAuthorizationResources(mock, nil, nil)

	inventory, err := newService(db, workos, &fakeOperationStore{}).Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if len(inventory.Resources) != 2 {
		t.Fatalf("resources = %+v", inventory.Resources)
	}
	sort.Strings(workos.listedOrgIDs)
	if !reflect.DeepEqual(workos.listedOrgIDs, []string{"org_a", "org_b"}) {
		t.Fatalf("listed organizations = %v", workos.listedOrgIDs)
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
	store := &fakeOperationStore{operation: &Operation{ID: "op_123", AccountID: "acct_123", ConfirmedCount: &confirmed}}
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

func TestResetRetryRechecksCountBeforeAnyDeletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for range 2 {
		mock.ExpectQuery(`SELECT COALESCE\(ao.workos_org_id, ''\)`).
			WithArgs("acct_123").
			WillReturnRows(sqlmock.NewRows([]string{"workos_org_id"}).AddRow("org_123"))
	}

	confirmed := 1
	workos := &fakeWorkOS{
		FakeFGA: &authz.FakeFGA{
			ListRoleAssignmentsFunc:      func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) { return nil, nil },
			ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) { return nil, nil },
		},
		FakeGroups: &authz.FakeGroups{
			ListGroupsFunc: func(context.Context, string, authz.PageRequest) (authz.GroupPage, error) {
				return authz.GroupPage{}, nil
			},
		},
		listResourcesErr: errors.New("temporary WorkOS failure"),
	}
	store := &fakeOperationStore{operation: &Operation{ID: "op_123", AccountID: "acct_123", ConfirmedCount: &confirmed}}
	service := newService(db, workos, store)

	if err := service.RunReset(context.Background(), "op_123"); err == nil {
		t.Fatal("first RunReset() error = nil, want WorkOS failure")
	}
	workos.listResourcesErr = nil
	workos.resources = []authz.AuthorizationResource{
		{ID: "dep_1", OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_1")},
		{ID: "dep_2", OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_2")},
	}
	if err := service.RunReset(context.Background(), "op_123"); err == nil {
		t.Fatal("retry RunReset() error = nil, want confirmed-count mismatch")
	}
	if len(workos.deleted) != 0 {
		t.Fatalf("deleted resources = %v, want none", workos.deleted)
	}
	if store.operation.AttemptCount != 2 || store.operation.SucceededCount != 0 {
		t.Fatalf("operation = %+v", store.operation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
