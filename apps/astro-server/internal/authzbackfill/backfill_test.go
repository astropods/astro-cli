package authzbackfill

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type fakeSource struct {
	blueprintIDs int
	accounts     []Account
	resources    map[string][]Resource
}

func (f *fakeSource) BackfillBlueprintIDs(context.Context, int, bool) (int, error) {
	return f.blueprintIDs, nil
}

func (f *fakeSource) ListAccounts(_ context.Context, after string, _ int) ([]Account, error) {
	if after != "" {
		return nil, nil
	}
	return f.accounts, nil
}

func (f *fakeSource) ListResources(context.Context, []string) (map[string][]Resource, error) {
	return f.resources, nil
}

type registration struct {
	organizationID string
	resource       authz.ResourceRef
	parent         authz.ResourceRef
	name           string
}

type fakeWorkOS struct {
	resources     []authz.AuthorizationResource
	registrations []registration
	assignments   []authz.RoleAssignment
	assignErr     error
}

func (f *fakeWorkOS) RegisterResourceWithParent(_ context.Context, organizationID string, resource, parent authz.ResourceRef, name string) error {
	f.registrations = append(f.registrations, registration{organizationID, resource, parent, name})
	return nil
}

func (f *fakeWorkOS) GetResource(_ context.Context, organizationID string, resource authz.ResourceRef) (authz.AuthorizationResource, error) {
	return authz.AuthorizationResource{ID: "workos_account", OrganizationID: organizationID, Resource: resource}, nil
}

func (f *fakeWorkOS) ListAuthorizationResourcesForOrganization(context.Context, string) ([]authz.AuthorizationResource, error) {
	return f.resources, nil
}

func (f *fakeWorkOS) AssignRole(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
	if f.assignErr != nil {
		return f.assignErr
	}
	f.assignments = append(f.assignments, authz.RoleAssignment{Subject: subject, Role: role, Resource: resource})
	return nil
}

func TestBackfillerCreatesMissingHierarchyAndAssignsOwner(t *testing.T) {
	t.Parallel()

	account := Account{ID: "account_123", OrganizationID: "org_123", Name: "Support", OwnerMembershipID: "om_owner"}
	source := &fakeSource{
		blueprintIDs: 2,
		accounts:     []Account{account},
		resources: map[string][]Resource{account.ID: {
			{AccountID: account.ID, Ref: authz.BlueprintResource("blueprint_123"), Name: "Support agent"},
			{AccountID: account.ID, Ref: authz.DeploymentResource("dep_123"), Name: "Support production"},
		}},
	}
	workos := &fakeWorkOS{}

	summary, err := New(source, workos, 10, false).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v, failures = %+v", err, summary.Failures)
	}
	if summary.BlueprintIDsBackfilled != 2 || summary.ResourcesMissing != 3 || summary.ResourcesCreated != 3 || summary.AdminsAssigned != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	want := []registration{
		{"org_123", authz.AccountResource("account_123"), authz.OrganizationResource("org_123"), "Support"},
		{"org_123", authz.BlueprintResource("blueprint_123"), authz.AccountResource("account_123"), "Support agent"},
		{"org_123", authz.DeploymentResource("dep_123"), authz.AccountResource("account_123"), "Support production"},
	}
	if !reflect.DeepEqual(workos.registrations, want) {
		t.Fatalf("registrations = %+v, want %+v", workos.registrations, want)
	}
	if len(workos.assignments) != 1 || workos.assignments[0].Role != authz.RoleAccountAdmin || workos.assignments[0].Subject.ID != "om_owner" {
		t.Fatalf("assignments = %+v", workos.assignments)
	}
}

func TestBackfillerKeepsExistingResourcesAndAdmin(t *testing.T) {
	t.Parallel()

	account := Account{ID: "account_123", OrganizationID: "org_123", Name: "Support", OwnerMembershipID: "om_owner"}
	accountResource := authz.AuthorizationResource{ID: "workos_account", Resource: authz.AccountResource(account.ID)}
	deploymentResource := authz.AuthorizationResource{ID: "workos_dep", ParentResourceID: "workos_account", Resource: authz.DeploymentResource("dep_123")}
	workos := &fakeWorkOS{resources: []authz.AuthorizationResource{accountResource, deploymentResource}, assignErr: authz.ErrRoleAssignmentExists}
	source := &fakeSource{accounts: []Account{account}, resources: map[string][]Resource{account.ID: {{AccountID: account.ID, Ref: authz.DeploymentResource("dep_123"), Name: "Support production"}}}}

	summary, err := New(source, workos, 10, false).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if summary.ResourcesExisting != 2 || summary.ResourcesCreated != 0 || summary.AdminsExisting != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestBackfillerReportsWrongParent(t *testing.T) {
	t.Parallel()

	account := Account{ID: "account_123", OrganizationID: "org_123", Name: "Support", OwnerMembershipID: "om_owner"}
	workos := &fakeWorkOS{resources: []authz.AuthorizationResource{
		{ID: "workos_account", Resource: authz.AccountResource(account.ID)},
		{ID: "workos_dep", ParentResourceID: "wrong_parent", Resource: authz.DeploymentResource("dep_123")},
	}}
	source := &fakeSource{accounts: []Account{account}, resources: map[string][]Resource{account.ID: {{AccountID: account.ID, Ref: authz.DeploymentResource("dep_123"), Name: "Support"}}}}

	summary, err := New(source, workos, 10, false).Run(context.Background())
	if !errors.Is(err, ErrIncomplete) || len(summary.Failures) != 1 || summary.Failures[0].Operation != "validate parent" {
		t.Fatalf("Run() = summary %+v, error %v", summary, err)
	}
}

func TestBackfillerDryRunDoesNotWrite(t *testing.T) {
	t.Parallel()

	account := Account{ID: "account_123", OrganizationID: "org_123", Name: "Support", OwnerMembershipID: "om_owner"}
	workos := &fakeWorkOS{}
	source := &fakeSource{accounts: []Account{account}, resources: map[string][]Resource{account.ID: {{AccountID: account.ID, Ref: authz.DeploymentResource("dep_123"), Name: "Support production"}}}}

	summary, err := New(source, workos, 10, true).Run(context.Background())
	if err != nil || summary.ResourcesMissing != 2 || len(workos.registrations) != 0 || len(workos.assignments) != 0 {
		t.Fatalf("Run() = summary %+v, error %v", summary, err)
	}
}

func TestBackfillerReportsMissingOwnerMembership(t *testing.T) {
	t.Parallel()

	account := Account{ID: "account_123", OrganizationID: "org_123", Name: "Support"}
	workos := &fakeWorkOS{resources: []authz.AuthorizationResource{{ID: "workos_account", Resource: authz.AccountResource(account.ID)}}}
	source := &fakeSource{accounts: []Account{account}, resources: map[string][]Resource{}}

	summary, err := New(source, workos, 10, false).Run(context.Background())
	if !errors.Is(err, ErrIncomplete) || len(summary.Failures) != 1 || summary.Failures[0].Operation != "assign admin" {
		t.Fatalf("Run() = summary %+v, error %v", summary, err)
	}
}
