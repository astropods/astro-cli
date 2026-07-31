package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
)

func TestApplyAccountClusterPlacement(t *testing.T) {
	clusterID := "eu-west-1-managed"
	ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}

	applyAccountClusterPlacement(ds, &account.Account{ClusterID: &clusterID})
	if ds.Target.ClusterID != clusterID {
		t.Fatalf("cluster_id = %q, want %q", ds.Target.ClusterID, clusterID)
	}

	applyAccountClusterPlacement(ds, &account.Account{})
	if ds.Target.ClusterID != "" {
		t.Fatalf("cluster_id = %q, want empty for primary", ds.Target.ClusterID)
	}
}
