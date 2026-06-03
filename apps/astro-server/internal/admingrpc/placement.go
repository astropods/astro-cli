package admingrpc

import (
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
)

func placementMismatch(accountClusterID, deploymentClusterID string) bool {
	return clusterplacement.PlacementMismatch(accountClusterID, deploymentClusterID)
}

func clusterIDLabel(id string) string {
	return clusterplacement.ClusterIDLabel(id)
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
		"Account is pinned to %q but this deployment routes to %q. Queen Redeploy queues teardown on the source cluster, then redeploys to the account cluster.",
		clusterIDLabel(accountClusterID),
		clusterIDLabel(deploymentClusterID),
	)
}

func patchDeploymentSpecClusterID(specJSON, clusterID string) (string, error) {
	return clusterplacement.PatchDeploymentSpecClusterID(specJSON, clusterID)
}

func placementUpdateMessage(fromClusterID, toClusterID string) string {
	return "Admin re-apply: " + clusterplacement.MigrationEventMessage(fromClusterID, toClusterID)
}
