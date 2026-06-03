package clusterplacement

import (
	"strings"
	"testing"
)

func TestNormalizedClusterID(t *testing.T) {
	if got := NormalizedClusterID(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
	if got := NormalizedClusterID("primary"); got != "" {
		t.Fatalf("primary sentinel: got %q", got)
	}
	if got := NormalizedClusterID("eu-west-1"); got != "eu-west-1" {
		t.Fatalf("eu-west-1: got %q", got)
	}
}

func TestPlacementMismatch(t *testing.T) {
	if PlacementMismatch("", "") {
		t.Fatal("both primary: expected no mismatch")
	}
	if PlacementMismatch("eu", "eu") {
		t.Fatal("both eu: expected no mismatch")
	}
	if !PlacementMismatch("eu", "") {
		t.Fatal("account eu, deployment primary: expected mismatch")
	}
	if !PlacementMismatch("", "eu") {
		t.Fatal("account primary, deployment eu: expected mismatch")
	}
}

func TestPatchDeploymentSpecClusterID(t *testing.T) {
	specJSON := `{"target":{"runtime":"kubernetes"},"workloads":[]}`
	got, err := PatchDeploymentSpecClusterID(specJSON, "eu")
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !strings.Contains(got, `"cluster_id":"eu"`) {
		t.Fatalf("expected eu cluster_id in spec: %s", got)
	}
	gotPrimary, err := PatchDeploymentSpecClusterID(got, "")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if strings.Contains(gotPrimary, "cluster_id") {
		t.Fatalf("expected cleared cluster_id: %s", gotPrimary)
	}
}

func TestMigrationEventMessage(t *testing.T) {
	got := MigrationEventMessage("", "eu")
	if !strings.Contains(got, "primary") || !strings.Contains(got, "eu") {
		t.Fatalf("unexpected message: %q", got)
	}
}
