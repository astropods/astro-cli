package admingrpc

import (
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func TestPlacementHintMessage(t *testing.T) {
	if got := placementHintMessage([]string{"cluster-a"}, "cluster-a", clusterid.New("cluster-default")); got != "" {
		t.Fatalf("allowed cluster: got %q", got)
	}
	if got := placementHintMessage([]string{"cluster-default"}, "", clusterid.New("cluster-default")); got != "" {
		t.Fatalf("bound primary: got %q", got)
	}
	got := placementHintMessage([]string{"cluster-a"}, "cluster-b", clusterid.New("cluster-default"))
	if got == "" {
		t.Fatal("expected hint for an orphaned deployment")
	}
	for _, part := range []string{"cluster-b", "cluster-a", "Migrate"} {
		if !strings.Contains(got, part) {
			t.Fatalf("hint missing %q: %q", part, got)
		}
	}
}

func TestPlacementOrphaned(t *testing.T) {
	if placementOrphaned([]string{"cluster-a"}, "cluster-a") {
		t.Error("allowed cluster should not be orphaned")
	}
	// An account with no bindings is unrestricted, not restricted to nothing.
	if placementOrphaned(nil, "cluster-b") {
		t.Error("an unbound account's deployment should not be flagged")
	}
	// A row the backfill has not reached yet records no cluster. Guessing which
	// one it means would flag every such row for the length of the gap.
	if placementOrphaned([]string{"cluster-a"}, "") {
		t.Error("a deployment with no cluster recorded should not be flagged")
	}
	if !placementOrphaned([]string{"cluster-a"}, "cluster-b") {
		t.Error("unbound cluster should be orphaned")
	}
	// The set is exhaustive, so the primary is no exception to it.
	if !placementOrphaned([]string{"cluster-a"}, "cluster-default") {
		t.Error("a deployment on an unbound primary is orphaned")
	}
	// No reserved word names a cluster, so "default" is an ordinary id.
	if !placementOrphaned([]string{"cluster-a"}, "default") {
		t.Error(`a deployment on a cluster named "default" with no binding is orphaned`)
	}
}

func TestPlacementUpdateMessage(t *testing.T) {
	// No registry configured, so no primary: an unrecorded target has no id to name.
	got := (&Server{}).placementUpdateMessage("", "cluster-a")
	if !strings.Contains(got, "unrecorded") || !strings.Contains(got, "cluster-a") {
		t.Fatalf("unexpected message: %q", got)
	}
	if !strings.Contains(got, "Admin re-apply") {
		t.Fatalf("expected admin prefix: %q", got)
	}
}
