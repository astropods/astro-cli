package admingrpc

import (
	"context"
	"errors"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
)

// ListAccountClusters returns the clusters an account may deploy to.
func (s *Server) ListAccountClusters(ctx context.Context, req *adminv1.ListAccountClustersRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	return s.accountClusterList(req.AccountID)
}

// AddAccountCluster allows a cluster for an account, optionally as its default.
func (s *Server) AddAccountCluster(ctx context.Context, req *adminv1.AddAccountClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.Add(req.AccountID, req.ClusterID, req.SetDefault); err != nil {
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin allowed cluster "+req.ClusterID+" for account",
		map[string]any{"cluster_id": req.ClusterID, "set_default": req.SetDefault})

	return s.accountClusterList(req.AccountID)
}

// RemoveAccountCluster refuses while the account still has deployments there.
func (s *Server) RemoveAccountCluster(ctx context.Context, req *adminv1.RemoveAccountClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.Remove(req.AccountID, req.ClusterID); err != nil {
		if errors.Is(err, account.ErrClusterInUse) {
			return nil, fmt.Errorf("%w: move or undeploy them before unbinding it", err)
		}
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin disallowed cluster "+req.ClusterID+" for account",
		map[string]any{"cluster_id": req.ClusterID})

	return s.accountClusterList(req.AccountID)
}

// SetAccountDefaultCluster moves the default flag to an already-allowed cluster.
func (s *Server) SetAccountDefaultCluster(ctx context.Context, req *adminv1.SetAccountDefaultClusterRequest) (*adminv1.AccountClusterList, error) {
	if req.AccountID == "" || req.ClusterID == "" {
		return nil, fmt.Errorf("account_id and cluster_id are required")
	}

	if err := s.bindings.SetDefault(req.AccountID, req.ClusterID); err != nil {
		return nil, err
	}

	s.logAccountClusterAudit(req.AccountID, auditlog.AccountSetCluster,
		"Admin set default cluster to "+req.ClusterID,
		map[string]any{"cluster_id": req.ClusterID})

	return s.accountClusterList(req.AccountID)
}

func (s *Server) accountClusterList(accountID string) (*adminv1.AccountClusterList, error) {
	allowed, err := s.bindings.List(accountID)
	if err != nil {
		return nil, err
	}
	out := &adminv1.AccountClusterList{}
	for _, b := range allowed {
		out.Clusters = append(out.Clusters, &adminv1.AccountCluster{
			ClusterID:   b.ClusterID,
			Region:      b.Region,
			RegionLabel: b.RegionLabel,
			RegionFlag:  b.RegionFlag,
			IsDefault:   b.IsDefault,
		})
	}
	return out, nil
}

func (s *Server) logAccountClusterAudit(accountID, action, description string, metadata map[string]any) {
	if s.auditStore == nil {
		return
	}
	evt := auditlog.ForAdmin(accountID, "grpc")
	evt.Action = action
	evt.ResourceType = "account"
	evt.ResourceID = accountID
	evt.Description = description
	evt.Metadata = metadata
	s.auditStore.LogAsync(s.log, evt)
}
