package admingrpc

import (
	"context"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
)

// SetAccountCluster assigns or clears the additional cluster placement for an account.
func (s *Server) SetAccountCluster(ctx context.Context, req *adminv1.SetAccountClusterRequest) (*adminv1.SetAccountClusterResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	clusterID := req.ClusterID
	if clusterID != "" {
		if s.clusterStore == nil {
			return nil, fmt.Errorf("cluster store not configured")
		}
		row, err := s.clusterStore.Get(ctx, clusterID)
		if err != nil {
			return nil, clusterStoreErr(err)
		}
		if !row.Enabled {
			return nil, fmt.Errorf("cluster %q is disabled", clusterID)
		}
	}

	store := account.NewAccountStore(s.db)
	if err := store.SetClusterID(req.AccountID, clusterID); err != nil {
		return nil, err
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.AccountSetCluster
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		if clusterID == "" {
			evt.Description = "Admin cleared account cluster placement (primary)"
		} else {
			evt.Description = "Admin set account cluster placement to " + clusterID
		}
		evt.Metadata = map[string]any{"cluster_id": clusterID}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.SetAccountClusterResponse{
		Status:    "updated",
		ClusterID: clusterID,
	}, nil
}
