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

type recordingResourceRegistrar struct {
	registrations []resourceRegistration
}

func (r *recordingResourceRegistrar) RegisterResourceWithParent(
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

func TestRegisterAccountAuthorizationResources(t *testing.T) {
	t.Parallel()

	registrar := &recordingResourceRegistrar{}
	acct := &account.Account{
		ID:                   "account_123",
		Name:                 "support",
		DisplayName:          "Support",
		Type:                 "organization",
		WorkOSOrganizationID: "org_123",
	}

	registerAccountAuthorizationResources(context.Background(), logger.New("error", "json"), registrar, acct)

	want := []resourceRegistration{
		{
			organizationID: "org_123",
			resource:       authz.AccountResource("account_123"),
			parent:         authz.OrganizationResource("org_123"),
			name:           "Support",
		},
		{
			organizationID: "org_123",
			resource:       authz.InsightsResource("account_123"),
			parent:         authz.AccountResource("account_123"),
			name:           "Insights",
		},
	}
	if !reflect.DeepEqual(registrar.registrations, want) {
		t.Fatalf("registrations = %#v, want %#v", registrar.registrations, want)
	}
}

func TestRegisterAuthorizationResourceSkipsPersonalAccounts(t *testing.T) {
	t.Parallel()

	registrar := &recordingResourceRegistrar{}
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
