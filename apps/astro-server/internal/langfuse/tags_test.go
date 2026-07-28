package langfuse

import "testing"

func TestHasDeploymentTag(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		deploymentID string
		want         bool
	}{
		{name: "matches", tags: []string{"env:prod", "deployment:dep-1"}, deploymentID: "dep-1", want: true},
		{name: "no match", tags: []string{"deployment:other"}, deploymentID: "dep-1"},
		{name: "empty tags", deploymentID: "dep-1"},
		{name: "prefix only", tags: []string{"deployment:"}, deploymentID: "dep-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasDeploymentTag(tt.tags, tt.deploymentID); got != tt.want {
				t.Fatalf("HasDeploymentTag(%v, %q) = %v, want %v", tt.tags, tt.deploymentID, got, tt.want)
			}
		})
	}
}
