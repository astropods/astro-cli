package watcher

import "testing"

func TestEnrollsOnlyForUserDeploymentActions(t *testing.T) {
	cases := []struct {
		name         string
		action       string
		resourceType string
		actorType    string
		want         bool
	}{
		{"deploy by a member", "deployment.deploy", "deployment", "user", true},
		{"rollback by a member", "deployment.rollback", "deployment", "user", true},
		{"a future deployment action", "deployment.scale", "deployment", "user", true},

		// Automation and operators act on deployments they do not own; enrolling
		// them would mail an admin every alert for every account they touch.
		{"admin action", "deployment.restart", "deployment", "admin", false},
		{"system action", "deployment.deploy", "deployment", "system", false},

		// Same actor, different resource — agent-level actions are not deployment
		// subscriptions, and resource_id would not be a deployment id.
		{"agent action", "agent.register", "agent", "user", false},
		{"account action", "account.rename", "account", "user", false},
		// Resource type is what makes resource_id a deployment id; a matching
		// action prefix alone is not enough.
		{"deployment action on another resource", "deployment.deploy", "agent", "user", false},
	}

	for _, tc := range cases {
		if got := Enrolls(tc.action, tc.resourceType, tc.actorType); got != tc.want {
			t.Errorf("%s: Enrolls(%q, %q, %q) = %t, want %t",
				tc.name, tc.action, tc.resourceType, tc.actorType, got, tc.want)
		}
	}
}
