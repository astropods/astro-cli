package handlers

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro-spec"
)

// Redeploy prefill must preserve the custom interface's public state. A
// custom-interface-only agent's freshly generated base template has no
// interfaces block, so the stored auth (incl. custom.public) has to be restored
// onto a created block — otherwise the public flag is silently reset on redeploy.
func TestMergeDeploymentPrefill_CustomPublicPreserved(t *testing.T) {
	stored := spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Auth: &spec.DeploymentInterfacesAuth{
				Custom: &spec.DeploymentCustomAuth{Public: true},
			},
		},
	}
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored spec: %v", err)
	}

	existing := &deploymentstore.Deployment{
		ID:                 "dep-1",
		AccountID:          "acct-1",
		DisplayName:        "My Agent",
		DeploymentSpecJSON: string(storedJSON),
	}

	// A freshly generated frontend/custom-only template has no interfaces block.
	template := &spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Image: "registry.example.com/my-agent:abc",
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 80, Protocol: "http", Expose: &spec.EndpointExpose{Enabled: true}},
			},
		},
	}

	// Stores backed by an empty sqlmock: GetByID / GetAgentProvisioning return
	// errors, which mergeDeploymentPrefill handles gracefully; a nil authzStore
	// skips grant restoration so the test isolates the auth/public copy.
	db, _, _ := sqlmock.New()
	defer db.Close()
	accountStore := account.NewAccountStore(db)
	deployStore := deploymentstore.NewStore(db)
	log := logger.New("error", "text")

	mergeDeploymentPrefill(log, template, existing, accountStore, deployStore, nil)

	if template.Interfaces == nil || template.Interfaces.Auth == nil || template.Interfaces.Auth.Custom == nil {
		t.Fatalf("expected interfaces.auth.custom restored on redeploy, got %+v", template.Interfaces)
	}
	if !template.Interfaces.Auth.Custom.Public {
		t.Error("custom.public should be preserved as true across redeploy prefill")
	}
}

// The same round-trip for the messaging web surface: web.public survives a
// redeploy of an agent that already has a messaging interfaces block.
func TestMergeDeploymentPrefill_WebPublicPreserved(t *testing.T) {
	stored := spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"web"},
			Auth: &spec.DeploymentInterfacesAuth{
				Web: &spec.DeploymentWebAuth{Type: "oidc", Public: true, Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}}},
			},
		},
	}
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored spec: %v", err)
	}

	existing := &deploymentstore.Deployment{
		ID:                 "dep-2",
		AccountID:          "acct-1",
		DeploymentSpecJSON: string(storedJSON),
	}

	// A messaging agent's base template already carries an interfaces block.
	template := &spec.AstroDeploymentSpec{
		Interfaces: &spec.DeploymentInterfaces{
			Image:    "registry.example.com/messaging:latest",
			Adapters: []string{"web"},
			Auth:     &spec.DeploymentInterfacesAuth{Web: &spec.DeploymentWebAuth{Type: "oidc"}},
		},
	}

	db, _, _ := sqlmock.New()
	defer db.Close()
	accountStore := account.NewAccountStore(db)
	deployStore := deploymentstore.NewStore(db)
	log := logger.New("error", "text")

	mergeDeploymentPrefill(log, template, existing, accountStore, deployStore, nil)

	if template.Interfaces == nil || template.Interfaces.Auth == nil || template.Interfaces.Auth.Web == nil {
		t.Fatalf("expected interfaces.auth.web restored, got %+v", template.Interfaces)
	}
	if !template.Interfaces.Auth.Web.Public {
		t.Error("web.public should be preserved as true across redeploy prefill")
	}
}
