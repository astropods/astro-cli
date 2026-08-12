package authz

import (
	"strings"
	"testing"
)

func TestDeploymentActionsAreUniqueExternalSlugs(t *testing.T) {
	seen := make(map[Action]struct{})
	for _, action := range DeploymentActions() {
		if !strings.HasPrefix(string(action), "deployment:") {
			t.Errorf("invalid deployment action %q", action)
		}
		if _, exists := seen[action]; exists {
			t.Errorf("duplicate deployment action %q", action)
		}
		seen[action] = struct{}{}
	}
}

func TestDeploymentActionsReturnsCopy(t *testing.T) {
	actions := DeploymentActions()
	actions[0] = "changed"
	if DeploymentActions()[0] == "changed" {
		t.Fatal("DeploymentActions returned mutable catalog storage")
	}
}
