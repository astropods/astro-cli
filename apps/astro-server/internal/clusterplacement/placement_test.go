package clusterplacement

import (
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
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
	if got := MigrationEventMessage("", "eu", clusterid.Resolver{}); !strings.Contains(got, "unrecorded") {
		t.Fatalf("unexpected message: %q", got)
	}
}

func deploymentOn(clusterID string) *deploymentstore.Deployment {
	return &deploymentstore.Deployment{ClusterID: &clusterID}
}

func pendingMoveTo(rowCluster, specCluster string) *deploymentstore.Deployment {
	dep := deploymentOn(rowCluster)
	dep.Status = deploymentstore.StatusPending
	dep.DeploymentSpecJSON = `{"target":{"runtime":"kubernetes","cluster_id":"` + specCluster + `"}}`
	return dep
}

func TestInFlightMove(t *testing.T) {
	clusters := clusterid.New("cluster-primary")

	tests := []struct {
		name string
		dep  *deploymentstore.Deployment
		want string
	}{
		{name: "no deployment is not moving"},
		{
			name: "pending with a different target in the spec is moving",
			dep:  pendingMoveTo("cluster-a", "cluster-b"),
			want: "cluster-b",
		},
		{
			name: "pending with a matching target is a plain redeploy",
			dep:  pendingMoveTo("cluster-a", "cluster-a"),
		},
		{
			name: "an unrecorded target on the primary is not moving",
			dep:  pendingMoveTo("cluster-primary", ""),
		},
		{
			name: "an active deployment is settled, whatever its spec says",
			dep: func() *deploymentstore.Deployment {
				dep := pendingMoveTo("cluster-a", "cluster-b")
				dep.Status = deploymentstore.StatusActive
				return dep
			}(),
		},
		{
			name: "unparseable spec reads as not moving rather than blocking a deploy",
			dep: func() *deploymentstore.Deployment {
				dep := pendingMoveTo("cluster-a", "cluster-b")
				dep.DeploymentSpecJSON = "{not json"
				return dep
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := InFlightMove(tc.dep, clusters); got != tc.want {
				t.Fatalf("inFlightMove = %q, want %q", got, tc.want)
			}
		})
	}
}
