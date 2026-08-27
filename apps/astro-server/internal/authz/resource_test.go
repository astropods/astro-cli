package authz_test

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestAuthorizationResourceRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  authz.ResourceRef
		want authz.ResourceRef
	}{
		{"account", authz.AccountResource("account_123"), authz.ResourceRef{Type: authz.ResourceAccount, ExternalID: "account_123"}},
		{"audience", authz.AudienceResource("audience_123"), authz.ResourceRef{Type: authz.ResourceAudience, ExternalID: "audience_123"}},
		{"blueprint", authz.BlueprintResource("blueprint_123"), authz.ResourceRef{Type: authz.ResourceBlueprint, ExternalID: "blueprint_123"}},
		{"deployment", authz.DeploymentResource("dep_123"), authz.ResourceRef{Type: authz.ResourceDeployment, ExternalID: "dep_123"}},
		{"insights", authz.InsightsResource("account_123"), authz.ResourceRef{Type: authz.ResourceInsights, ExternalID: "account_123"}},
		{"knowledge store", authz.KnowledgeStoreResource("ks_123"), authz.ResourceRef{Type: authz.ResourceKnowledge, ExternalID: "ks_123"}},
		{"organization", authz.OrganizationResource("org_123"), authz.ResourceRef{Type: authz.ResourceOrganization, ExternalID: "org_123"}},
		{"variable", authz.VariableResource("account_123", "API_KEY"), authz.ResourceRef{Type: authz.ResourceVariable, ExternalID: "account_123:API_KEY"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.got != test.want {
				t.Fatalf("resource = %#v, want %#v", test.got, test.want)
			}
		})
	}
}
