package admingrpc

import (
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
)

func TestPlacementHintMessage(t *testing.T) {
	if got := placementHintMessage("eu", "eu", clusterid.New("primary")); got != "" {
		t.Fatalf("no mismatch: got %q", got)
	}
	got := placementHintMessage("eu", "", clusterid.New("primary"))
	if got == "" {
		t.Fatal("expected hint")
	}
	for _, part := range []string{"eu", "primary", "Redeploy"} {
		if !strings.Contains(got, part) {
			t.Fatalf("hint missing %q: %q", part, got)
		}
	}
}

func TestPlacementUpdateMessage(t *testing.T) {
	got := (&Server{}).placementUpdateMessage("", "eu")
	if !strings.Contains(got, "unrecorded") || !strings.Contains(got, "eu") {
		t.Fatalf("unexpected message: %q", got)
	}
	if !strings.Contains(got, "Admin re-apply") {
		t.Fatalf("expected admin prefix: %q", got)
	}
}

func TestPatchDeploymentSpecClusterIDDelegates(t *testing.T) {
	specJSON := `{"target":{"runtime":"kubernetes"},"workloads":[]}`
	got, err := (&Server{}).patchDeploymentSpecClusterID(specJSON, "eu")
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	want, err := clusterplacement.PatchDeploymentSpecClusterID(specJSON, "eu", clusterid.Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
