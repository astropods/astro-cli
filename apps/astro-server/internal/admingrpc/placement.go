package admingrpc

import (
	"encoding/json"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	spec "github.com/astropods/astro/packages/astro-spec"
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
		"Account is pinned to %q but this deployment routes to %q. Queen Redeploy syncs routing to the account cluster before enqueueing; pods may stay on the old cluster until the deploy worker finishes.",
		clusterIDLabel(accountClusterID),
		clusterIDLabel(deploymentClusterID),
	)
}

// patchDeploymentSpecClusterID updates target.cluster_id in stored deployment spec JSON.
func patchDeploymentSpecClusterID(specJSON, clusterID string) (string, error) {
	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(specJSON), &ds); err != nil {
		return "", fmt.Errorf("parse deployment spec: %w", err)
	}
	ds.Target.ClusterID = clusterID
	out, err := json.Marshal(&ds)
	if err != nil {
		return "", fmt.Errorf("marshal deployment spec: %w", err)
	}
	return string(out), nil
}

func placementUpdateMessage(fromClusterID, toClusterID string) string {
	return fmt.Sprintf(
		"Admin re-apply: cluster placement updated from %s to %s",
		clusterIDLabel(fromClusterID),
		clusterIDLabel(toClusterID),
	)
}
