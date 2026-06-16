package authz_test

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestDeploymentResource(t *testing.T) {
	t.Parallel()

	got := authz.DeploymentResource("dep-abc")
	if got.Type != authz.ResourceDeployment {
		t.Fatalf("Type = %q, want %q", got.Type, authz.ResourceDeployment)
	}
	if got.ExternalID != "dep-abc" {
		t.Fatalf("ExternalID = %q, want %q", got.ExternalID, "dep-abc")
	}
}
