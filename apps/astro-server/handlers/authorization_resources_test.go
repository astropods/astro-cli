package handlers

import (
	"context"
	"reflect"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type resourceRegistration struct {
	organizationID string
	resource       authz.ResourceRef
	parent         authz.ResourceRef
	name           string
}

type recordingResourceLifecycle struct {
	registrations []resourceRegistration
	deletions     []resourceRegistration
}

func (r *recordingResourceLifecycle) RegisterResourceWithParent(
	_ context.Context,
	organizationID string,
	resource, parent authz.ResourceRef,
	name string,
) error {
	r.registrations = append(r.registrations, resourceRegistration{
		organizationID: organizationID,
		resource:       resource,
		parent:         parent,
		name:           name,
	})
	return nil
}

func (r *recordingResourceLifecycle) GetResource(context.Context, string, authz.ResourceRef) (authz.AuthorizationResource, error) {
	return authz.AuthorizationResource{}, nil
}

func (r *recordingResourceLifecycle) DeleteResource(_ context.Context, organizationID string, resource authz.ResourceRef) error {
	r.deletions = append(r.deletions, resourceRegistration{organizationID: organizationID, resource: resource})
	return nil
}

func TestRegisterAccountAuthorizationResource(t *testing.T) {
	t.Parallel()

	registrar := &recordingResourceLifecycle{}
	acct := &account.Account{
		ID:                   "account_123",
		Name:                 "support",
		DisplayName:          "Support",
		Type:                 "organization",
		WorkOSOrganizationID: "org_123",
	}

	registerAccountAuthorizationResource(context.Background(), logger.New("error", "json"), registrar, acct)

	want := []resourceRegistration{
		{
			organizationID: "org_123",
			resource:       authz.AccountResource("account_123"),
			parent:         authz.OrganizationResource("org_123"),
			name:           "Support",
		},
	}
	if !reflect.DeepEqual(registrar.registrations, want) {
		t.Fatalf("registrations = %#v, want %#v", registrar.registrations, want)
	}
}

func TestRegisterAuthorizationResourceSkipsPersonalAccounts(t *testing.T) {
	t.Parallel()

	registrar := &recordingResourceLifecycle{}
	registered := registerAuthorizationResource(
		context.Background(),
		logger.New("error", "json"),
		registrar,
		&account.Account{ID: "account_123", Type: "personal", WorkOSOrganizationID: "org_123"},
		authz.BlueprintResource("blueprint_123"),
		"Support agent",
	)
	if registered {
		t.Fatal("registerAuthorizationResource() = true, want false")
	}
	if len(registrar.registrations) != 0 {
		t.Fatalf("registrations = %#v, want none", registrar.registrations)
	}
}

func TestDeleteAuthorizationResource(t *testing.T) {
	t.Parallel()

	resources := &recordingResourceLifecycle{}
	acct := &account.Account{ID: "account_123", Type: "organization", WorkOSOrganizationID: "org_123"}
	resource := authz.DeploymentResource("deployment_123")

	deleteAuthorizationResource(context.Background(), logger.New("error", "json"), resources, acct, resource)

	wantDeletion := []resourceRegistration{{organizationID: "org_123", resource: resource}}
	if !reflect.DeepEqual(resources.deletions, wantDeletion) {
		t.Fatalf("deletions = %#v, want %#v", resources.deletions, wantDeletion)
	}
}

func TestDeleteAuthorizationResourceSkipsPersonalAccounts(t *testing.T) {
	t.Parallel()

	resources := &recordingResourceLifecycle{}
	acct := &account.Account{ID: "account_123", Type: "personal", WorkOSOrganizationID: "org_123"}
	resource := authz.DeploymentResource("deployment_123")

	deleteAuthorizationResource(context.Background(), logger.New("error", "json"), resources, acct, resource)

	if len(resources.deletions) != 0 {
		t.Fatalf("personal account lifecycle deletions = %#v, want none", resources.deletions)
	}
}
