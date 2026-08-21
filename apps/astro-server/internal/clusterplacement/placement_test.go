package clusterplacement

import (
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func TestPatchDeploymentSpecClusterID(t *testing.T) {
	specJSON := `{"target":{"runtime":"kubernetes"},"workloads":[]}`
	got, err := PatchDeploymentSpecClusterID(specJSON, "eu", clusterid.New("primary"))
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !strings.Contains(got, `"cluster_id":"eu"`) {
		t.Fatalf("expected eu cluster_id in spec: %s", got)
	}
	gotPrimary, err := PatchDeploymentSpecClusterID(got, "", clusterid.New("primary"))
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if !strings.Contains(gotPrimary, `"cluster_id":"primary"`) {
		t.Fatalf("expected the primary's id: %s", gotPrimary)
	}
}

func TestMigrationEventMessage(t *testing.T) {
	got := MigrationEventMessage("", "eu", clusterid.New("primary"))
	if !strings.Contains(got, "primary") || !strings.Contains(got, "eu") {
		t.Fatalf("unexpected message: %q", got)
	}
}
