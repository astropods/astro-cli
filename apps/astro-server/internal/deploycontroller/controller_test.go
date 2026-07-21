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
