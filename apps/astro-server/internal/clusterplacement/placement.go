package clusterplacement

import (
	"encoding/json"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
)

// PatchDeploymentSpecClusterID updates target.cluster_id in stored deployment spec JSON.
func PatchDeploymentSpecClusterID(specJSON, clusterID string, clusters clusterid.Resolver) (string, error) {
	var ds deployment.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(specJSON), &ds); err != nil {
		return "", fmt.Errorf("parse deployment spec: %w", err)
	}
	ds.Target.ClusterID = clusters.Canonical(clusterID)
	out, err := json.Marshal(&ds)
	if err != nil {
		return "", fmt.Errorf("marshal deployment spec: %w", err)
	}
	return string(out), nil
}

// MigrationEventMessage formats a cluster routing change for deployment_events.
// Queen ListClusterMigrations matches these message prefixes in SQL — keep in sync.
func MigrationEventMessage(fromClusterID, toClusterID string, clusters clusterid.Resolver) string {
	return fmt.Sprintf(
		"Cluster placement updated from %s to %s",
		clusters.Label(fromClusterID),
		clusters.Label(toClusterID),
	)
}

// AccountMigrationEventMessage formats an account-driven cluster migration event.
// Prefix is matched by admin ListClusterMigrations event queries.
func AccountMigrationEventMessage(fromClusterID, toClusterID string, clusters clusterid.Resolver) string {
	return "Account cluster migration: " + MigrationEventMessage(fromClusterID, toClusterID, clusters)
}
