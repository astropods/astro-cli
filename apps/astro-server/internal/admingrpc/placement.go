package admingrpc

import (
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// normalizedClusterID maps empty and the primary sentinel to "" so placement
// comparisons treat them as the same routing target.
func normalizedClusterID(id string) string {
	if id == "" || id == k8s.PrimaryClusterID {
		return ""
	}
	return id
}

// placementMismatch reports whether account placement and deployment routing
// target different clusters.
func placementMismatch(accountClusterID, deploymentClusterID string) bool {
	return normalizedClusterID(accountClusterID) != normalizedClusterID(deploymentClusterID)
}

func clusterIDLabel(id string) string {
	if id == "" {
		return "primary"
	}
	return id
}

// populateAdminDeploymentPlacement sets cluster placement fields on an admin deployment row.
func populateAdminDeploymentPlacement(ad *adminv1.AdminDeployment, deploymentClusterID, accountClusterID string) {
	if ad == nil {
		return
	}
	ad.ClusterID = deploymentClusterID
	ad.AccountClusterID = accountClusterID
	ad.PlacementMismatch = placementMismatch(accountClusterID, deploymentClusterID)
}

// placementHintMessage returns guidance when account and deployment clusters differ.
func placementHintMessage(accountClusterID, deploymentClusterID string) string {
	if !placementMismatch(accountClusterID, deploymentClusterID) {
		return ""
	}
	return fmt.Sprintf(
		"Account is pinned to %q but this deployment routes to %q. Redeploy does not change cluster; run a new deploy from the client.",
		clusterIDLabel(accountClusterID),
		clusterIDLabel(deploymentClusterID),
	)
}
