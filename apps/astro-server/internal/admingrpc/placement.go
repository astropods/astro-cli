package admingrpc

import (
	"fmt"
	"slices"
	"strings"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// Mirrors account.IsAllowed: an account with no bindings is unrestricted, and
// once it has any the set is exhaustive. The primary is no exception, so a
// deployment there is orphaned once the account stops allowing it. A row with no
// cluster recorded predates the backfill and is left alone rather than guessed at.
func placementOrphaned(accountClusterIDs []string, deploymentClusterID string) bool {
	if len(accountClusterIDs) == 0 || deploymentClusterID == "" {
		return false
	}
	return !slices.Contains(accountClusterIDs, deploymentClusterID)
}

// populateAdminDeploymentPlacement sets cluster placement fields on an admin
// deployment row. It takes the deployment rather than its parts so a new
// placement field reads what it needs without another argument.
func (s *Server) populateAdminDeploymentPlacement(ad *adminv1.AdminDeployment, dep *deploymentstore.Deployment, accountClusterIDs []string) {
	if ad == nil || dep == nil {
		return
	}
	ad.ClusterID = dep.EffectiveClusterID()
	ad.AccountClusterIDs = accountClusterIDs
	ad.MigratingToClusterID = clusterplacement.InFlightMove(dep, s.clusters)
	// An undeployed deployment has its cluster_id cleared (nothing runs there
	// anymore, nothing to redeploy) and would otherwise spuriously flag against
	// whatever clusters the account currently allows.
	if dep.Status != "undeployed" {
		ad.PlacementOrphaned = placementOrphaned(accountClusterIDs, ad.ClusterID)
	}
}

func placementHintMessage(accountClusterIDs []string, deploymentClusterID string, clusters clusterid.Resolver) string {
	if !placementOrphaned(accountClusterIDs, deploymentClusterID) {
		return ""
	}
	allowed := "none"
	if len(accountClusterIDs) > 0 {
		allowed = strings.Join(accountClusterIDs, ", ")
	}
	return fmt.Sprintf(
		"This deployment routes to %q, which the account no longer allows (allowed: %s). Migrate it onto an allowed cluster.",
		clusters.Label(deploymentClusterID),
		allowed,
	)
}

func (s *Server) placementUpdateMessage(fromClusterID, toClusterID string) string {
	return "Admin re-apply: " + clusterplacement.MigrationEventMessage(fromClusterID, toClusterID, s.clusters)
}
