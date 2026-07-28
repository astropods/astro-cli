package clusterplacement

import (
	"encoding/json"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	spec "github.com/astropods/astro-spec"
)

// NormalizedClusterID maps empty and the primary sentinel to "" so placement
// comparisons treat them as the same routing target.
func NormalizedClusterID(id string) string {
	if id == "" || id == k8s.PrimaryClusterID {
		return ""
	}
	return id
}

// PlacementMismatch reports whether two routing targets refer to different clusters.
func PlacementMismatch(targetClusterID, deploymentClusterID string) bool {
	return NormalizedClusterID(targetClusterID) != NormalizedClusterID(deploymentClusterID)
}

// ClusterIDLabel returns a human-readable cluster name for events and admin messages.
func ClusterIDLabel(id string) string {
	if id == "" {
		return "primary"
	}
	return id
}

// PatchDeploymentSpecClusterID updates target.cluster_id in stored deployment spec JSON.
func PatchDeploymentSpecClusterID(specJSON, clusterID string) (string, error) {
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

// MigrationEventMessage formats a cluster routing change for deployment_events.
// Queen ListClusterMigrations matches these message prefixes in SQL — keep in sync.
func MigrationEventMessage(fromClusterID, toClusterID string) string {
	return fmt.Sprintf(
		"Cluster placement updated from %s to %s",
		ClusterIDLabel(fromClusterID),
		ClusterIDLabel(toClusterID),
	)
}

// AccountMigrationEventMessage formats an account-driven cluster migration event.
// Prefix is matched by admin ListClusterMigrations event queries.
func AccountMigrationEventMessage(fromClusterID, toClusterID string) string {
	return "Account cluster migration: " + MigrationEventMessage(fromClusterID, toClusterID)
}
