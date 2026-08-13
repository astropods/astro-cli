package deploycontroller

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

func TestAggregateDeploymentPhase(t *testing.T) {
	expected := []*deploymentstore.Workload{{Name: "agent"}, {Name: "cache"}}

	ready := func(name string) deploymentstore.WorkloadStatus {
		return deploymentstore.WorkloadStatus{WorkloadName: name, Phase: deploymentstore.WorkloadPhaseReady}
	}

	tests := []struct {
		name      string
		observed  []deploymentstore.WorkloadStatus
		expected  []*deploymentstore.Workload
		wantPhase string
	}{
		{
			name:      "all declared ready → ready",
			observed:  []deploymentstore.WorkloadStatus{ready("agent"), ready("cache")},
			expected:  expected,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name:      "declared workload not yet observed → progressing",
			observed:  []deploymentstore.WorkloadStatus{ready("agent")},
			expected:  expected,
			wantPhase: deploymentstore.WorkloadPhaseProgressing,
		},
		{
			name: "declared workload observed but not ready → progressing",
			observed: []deploymentstore.WorkloadStatus{
				ready("agent"),
				{WorkloadName: "cache", Phase: deploymentstore.WorkloadPhaseProgressing},
			},
			expected:  expected,
			wantPhase: deploymentstore.WorkloadPhaseProgressing,
		},
		{
			name: "any workload failed → failed",
			observed: []deploymentstore.WorkloadStatus{
				ready("agent"),
				{WorkloadName: "cache", Phase: deploymentstore.WorkloadPhaseFailed, Reason: "ImagePullBackOff"},
			},
			expected:  expected,
			wantPhase: deploymentstore.WorkloadPhaseFailed,
		},
		{
			name: "completed job counts as settled → ready",
			observed: []deploymentstore.WorkloadStatus{
				ready("agent"),
				{WorkloadName: "cache", Phase: deploymentstore.WorkloadPhaseComplete},
			},
			expected:  expected,
			wantPhase: deploymentstore.WorkloadPhaseReady,
		},
		{
			name:      "no declared workloads → progressing (never vacuously active)",
			observed:  nil,
			expected:  nil,
			wantPhase: deploymentstore.WorkloadPhaseProgressing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := aggregateDeploymentPhase(tt.observed, tt.expected)
			if got != tt.wantPhase {
				t.Errorf("phase = %q, want %q", got, tt.wantPhase)
			}
		})
	}
}

// TestCanonicalCluster locks the default-cluster normalization: a
// default-cluster deployment's EffectiveClusterID ("") must compare equal to
// the registry's DefaultClusterID so the controller's cluster-ownership
// guard doesn't skip every default-cluster deployment.
func TestCanonicalCluster(t *testing.T) {
	c := &Controller{registry: k8s.NewRegistryForTest(nil, nil, k8s.RegistryConfig{DefaultClusterID: "default-eks"})}

	tests := []struct {
		in, want string
	}{
		{"", "default-eks"},
		{"default-eks", "default-eks"},
		{"cluster-abc", "cluster-abc"},
	}
	for _, tt := range tests {
		if got := c.canonicalCluster(tt.in); got != tt.want {
			t.Errorf("canonicalCluster(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	// The load-bearing property: the default cluster's two spellings match.
	if c.canonicalCluster("") != c.canonicalCluster("default-eks") {
		t.Error(`default "" and "default-eks" must canonicalize equal`)
	}
	// Distinct additional clusters don't collide with the default.
	if c.canonicalCluster("cluster-abc") == c.canonicalCluster("") {
		t.Error("additional cluster must not match default")
	}
}
