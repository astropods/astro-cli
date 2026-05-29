package admingrpc

import (
	"strings"
	"testing"
)

func TestNormalizedClusterID(t *testing.T) {
	if got := normalizedClusterID(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
	if got := normalizedClusterID("primary"); got != "" {
		t.Fatalf("primary sentinel: got %q", got)
	}
	if got := normalizedClusterID("eu-west-1"); got != "eu-west-1" {
		t.Fatalf("eu-west-1: got %q", got)
	}
}

func TestPlacementMismatch(t *testing.T) {
	if placementMismatch("", "") {
		t.Fatal("both primary: expected no mismatch")
	}
	if placementMismatch("eu", "eu") {
		t.Fatal("both eu: expected no mismatch")
	}
	if !placementMismatch("eu", "") {
		t.Fatal("account eu, deployment primary: expected mismatch")
	}
	if !placementMismatch("", "eu") {
		t.Fatal("account primary, deployment eu: expected mismatch")
	}
	if !placementMismatch("primary", "eu") {
		t.Fatal("account primary sentinel, deployment eu: expected mismatch")
	}
}

func TestPlacementHintMessage(t *testing.T) {
	if got := placementHintMessage("eu", "eu"); got != "" {
		t.Fatalf("no mismatch: got %q", got)
	}
	got := placementHintMessage("eu", "")
	if got == "" {
		t.Fatal("expected hint")
	}
	for _, part := range []string{"eu", "primary", "Redeploy"} {
		if !strings.Contains(got, part) {
			t.Fatalf("hint missing %q: %q", part, got)
		}
	}
}

func TestPatchDeploymentSpecClusterID(t *testing.T) {
	specJSON := `{"target":{"runtime":"kubernetes"},"workloads":[]}`
	got, err := patchDeploymentSpecClusterID(specJSON, "eu")
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !strings.Contains(got, `"cluster_id":"eu"`) {
		t.Fatalf("expected eu cluster_id in spec: %s", got)
	}
	gotPrimary, err := patchDeploymentSpecClusterID(got, "")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if strings.Contains(gotPrimary, "cluster_id") {
		t.Fatalf("expected cleared cluster_id: %s", gotPrimary)
	}
}

func TestPlacementUpdateMessage(t *testing.T) {
	got := placementUpdateMessage("", "eu")
	if !strings.Contains(got, "primary") || !strings.Contains(got, "eu") {
		t.Fatalf("unexpected message: %q", got)
	}
}
