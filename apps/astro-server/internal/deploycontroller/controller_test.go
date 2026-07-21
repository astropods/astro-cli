package deploycontroller

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// TestCanonicalCluster locks the primary-cluster normalization: a primary
// deployment's EffectiveClusterID ("") must compare equal to the registry's
// PrimaryClusterID ("primary") so the controller's cluster-ownership guard
// doesn't skip every primary-cluster deployment.
func TestCanonicalCluster(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", k8s.PrimaryClusterID},
		{k8s.PrimaryClusterID, k8s.PrimaryClusterID},
		{"cluster-abc", "cluster-abc"},
	}
	for _, tt := range tests {
		if got := canonicalCluster(tt.in); got != tt.want {
			t.Errorf("canonicalCluster(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	// The load-bearing property: primary's two spellings match.
	if canonicalCluster("") != canonicalCluster(k8s.PrimaryClusterID) {
		t.Error(`primary "" and "primary" must canonicalize equal`)
	}
	// Distinct additional clusters don't collide with primary.
	if canonicalCluster("cluster-abc") == canonicalCluster("") {
		t.Error("additional cluster must not match primary")
	}
}
