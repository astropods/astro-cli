package admingrpc

import (
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
)

func (s *Server) clusters() clusterid.Resolver {
	if s.k8sRegistry == nil {
		return clusterid.Resolver{}
	}
	return clusterid.New(s.k8sRegistry.DefaultClusterID())
}

// populateAdminDeploymentPlacement sets cluster placement fields on an admin deployment row.
func populateAdminDeploymentPlacement(ad *adminv1.AdminDeployment, deploymentClusterID, accountClusterID, status string, clusters clusterid.Resolver) {
	if ad == nil {
		return
	}
	ad.ClusterID = deploymentClusterID
	ad.AccountClusterID = accountClusterID
	// An undeployed deployment has its cluster_id cleared (nothing runs there
	// anymore, nothing to redeploy) and would otherwise spuriously mismatch
	// against whatever cluster the account is currently pinned to.
	if status != "undeployed" {
		ad.PlacementMismatch = !clusters.Same(accountClusterID, deploymentClusterID)
	}
}

// placementHintMessage returns guidance when account and deployment clusters differ.
func placementHintMessage(accountClusterID, deploymentClusterID string, clusters clusterid.Resolver) string {
	if clusters.Same(accountClusterID, deploymentClusterID) {
		return ""
	}
	return fmt.Sprintf(
		"Account is pinned to %q but this deployment routes to %q. Queen Redeploy queues teardown on the source cluster, then redeploys to the account cluster.",
		clusters.Label(accountClusterID),
		clusters.Label(deploymentClusterID),
	)
}

func (s *Server) patchDeploymentSpecClusterID(specJSON, clusterID string) (string, error) {
	return clusterplacement.PatchDeploymentSpecClusterID(specJSON, clusterID, s.clusters())
}

func (s *Server) placementUpdateMessage(fromClusterID, toClusterID string) string {
	return "Admin re-apply: " + clusterplacement.MigrationEventMessage(fromClusterID, toClusterID, s.clusters())
}
