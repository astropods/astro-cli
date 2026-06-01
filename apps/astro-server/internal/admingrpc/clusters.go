package admingrpc

import (
	"context"
	"errors"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

func (s *Server) requireClusterAdmin() error {
	if s.clusterStore == nil || s.k8sRegistry == nil {
		return status.Error(codes.FailedPrecondition, "kubernetes registry not configured")
	}
	return nil
}

func rejectPrimaryMutation(id string) error {
	if id == k8s.PrimaryClusterID {
		return status.Error(codes.InvalidArgument, "primary cluster is configured via env vars and cannot be changed via admin API")
	}
	return nil
}

func clusterStoreErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, clusterstore.ErrNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	if errors.Is(err, clusterstore.ErrAlreadyExists) {
		return status.Error(codes.AlreadyExists, err.Error())
	}
	if errors.Is(err, clusterstore.ErrInUse) {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Error(codes.InvalidArgument, err.Error())
}

func (s *Server) clusterHealth(ctx context.Context, entry k8s.ClusterEntry) (healthy bool, healthErr string) {
	if !entry.Enabled {
		return false, "cluster disabled"
	}
	var client k8s.ClusterClient
	if entry.IsPrimary {
		client = s.k8sRegistry.Default()
	} else {
		var err error
		client, err = s.k8sRegistry.Get(ctx, entry.ID)
		if err != nil {
			if errors.Is(err, k8s.ErrClusterDisabled) {
				return false, "cluster disabled"
			}
			return false, err.Error()
		}
	}
	if client == nil {
		return false, "kubernetes client not available"
	}
	if err := client.CheckHealth(); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func entryToProto(ctx context.Context, s *Server, entry k8s.ClusterEntry) *adminv1.RegisteredCluster {
	healthy, healthErr := s.clusterHealth(ctx, entry)
	out := &adminv1.RegisteredCluster{
		ID:                         entry.ID,
		Region:                     entry.Region,
		EKSClusterName:             entry.EKSClusterName,
		EKSClusterEndpoint:         entry.EKSClusterEndpoint,
		Enabled:                    entry.Enabled,
		IsPrimary:                  entry.IsPrimary,
		Healthy:                    healthy,
		HealthError:                healthErr,
		AgentIngressDomain:         entry.AgentIngressDomain,
		AgentACMCertificateARN:     entry.AgentACMCertARN,
		AgentALBGroupName:          entry.AgentALBGroupName,
		IngestionIngressDomain:     entry.IngestionIngressDomain,
		IngestionACMCertificateARN: entry.IngestionACMCertARN,
		IngestionALBGroupName:      entry.IngestionALBGroupName,
		KnowledgeDomain:            entry.KnowledgeDomain,
		LangfuseBaseURLExt:         entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:            entry.LangfuseVPCEIPs,
		PodSubnetCIDRs:             entry.PodSubnetCIDRs,
	}
	if !entry.IsPrimary {
		out.CreatedAt = entry.CreatedAt.UTC().Format(time.RFC3339)
		out.UpdatedAt = entry.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) rowToEntry(row *clusterstore.Cluster) k8s.ClusterEntry {
	return k8s.ClusterEntry{
		ID:                     row.ID,
		IsPrimary:              false,
		Region:                 row.Region,
		EKSClusterName:         row.EKSClusterName,
		EKSClusterEndpoint:     row.EKSClusterEndpoint,
		Enabled:                row.Enabled,
		AgentIngressDomain:     row.AgentIngressDomain,
		AgentACMCertARN:        row.AgentACMCertARN,
		AgentALBGroupName:      row.AgentALBGroupName,
		IngestionIngressDomain: row.IngestionIngressDomain,
		IngestionACMCertARN:    row.IngestionACMCertARN,
		IngestionALBGroupName:  row.IngestionALBGroupName,
		KnowledgeDomain:        row.KnowledgeDomain,
		LangfuseBaseURLExt:     row.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        row.LangfuseVPCEIPs,
		PodSubnetCIDRs:         row.PodSubnetCIDRs,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
}

func (s *Server) loadAdditionalEntry(ctx context.Context, id string) (k8s.ClusterEntry, error) {
	row, err := s.clusterStore.Get(ctx, id)
	if err != nil {
		return k8s.ClusterEntry{}, clusterStoreErr(err)
	}
	return s.rowToEntry(row), nil
}

// RegisterCluster inserts an additional cluster row and refreshes the registry cache.
func (s *Server) RegisterCluster(ctx context.Context, req *adminv1.RegisterClusterRequest) (*adminv1.RegisterClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := rejectPrimaryMutation(req.ID); err != nil {
		return nil, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	row := &clusterstore.Cluster{
		ID:                     req.ID,
		Region:                 req.Region,
		EKSClusterName:         req.EKSClusterName,
		EKSClusterEndpoint:     req.EKSClusterEndpoint,
		Enabled:                enabled,
		AgentIngressDomain:     req.AgentIngressDomain,
		AgentACMCertARN:        req.AgentACMCertificateARN,
		AgentALBGroupName:      req.AgentALBGroupName,
		IngestionIngressDomain: req.IngestionIngressDomain,
		IngestionACMCertARN:    req.IngestionACMCertificateARN,
		IngestionALBGroupName:  req.IngestionALBGroupName,
		KnowledgeDomain:        req.KnowledgeDomain,
		LangfuseBaseURLExt:     req.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        req.LangfuseVPCEIPs,
		PodSubnetCIDRs:         req.PodSubnetCIDRs,
	}
	if err := s.clusterStore.Register(ctx, row); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, row.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}

	stored, err := s.clusterStore.Get(ctx, row.ID)
	if err != nil {
		return nil, clusterStoreErr(err)
	}
	entry := s.rowToEntry(stored)
	return &adminv1.RegisterClusterResponse{Cluster: entryToProto(ctx, s, entry)}, nil
}

// EnableCluster sets enabled=true on an additional cluster.
func (s *Server) EnableCluster(ctx context.Context, req *adminv1.EnableClusterRequest) (*adminv1.EnableClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := rejectPrimaryMutation(req.ID); err != nil {
		return nil, err
	}
	if err := s.clusterStore.SetEnabled(ctx, req.ID, true); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}
	entry, err := s.loadAdditionalEntry(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &adminv1.EnableClusterResponse{Cluster: entryToProto(ctx, s, entry)}, nil
}

// DisableCluster sets enabled=false and evicts the cached client.
func (s *Server) DisableCluster(ctx context.Context, req *adminv1.DisableClusterRequest) (*adminv1.DisableClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := rejectPrimaryMutation(req.ID); err != nil {
		return nil, err
	}
	if err := s.clusterStore.SetEnabled(ctx, req.ID, false); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}
	entry, err := s.loadAdditionalEntry(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &adminv1.DisableClusterResponse{Cluster: entryToProto(ctx, s, entry)}, nil
}

// DeregisterCluster deletes an additional cluster row when no deployments reference it.
func (s *Server) DeregisterCluster(ctx context.Context, req *adminv1.DeregisterClusterRequest) (*adminv1.DeregisterClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := rejectPrimaryMutation(req.ID); err != nil {
		return nil, err
	}
	if err := s.clusterStore.Deregister(ctx, req.ID); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}
	return &adminv1.DeregisterClusterResponse{}, nil
}

// ListClusters returns the synthesized primary plus additional cluster rows.
func (s *Server) ListClusters(ctx context.Context, req *adminv1.ListClustersRequest) (*adminv1.ListClustersResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	entries, err := s.k8sRegistry.List(ctx, req.EnabledOnly)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
	}
	out := make([]*adminv1.RegisteredCluster, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entryToProto(ctx, s, entry))
	}
	return &adminv1.ListClustersResponse{Clusters: out}, nil
}

// UpdateCluster updates mutable fields on an additional cluster row.
func (s *Server) UpdateCluster(ctx context.Context, req *adminv1.UpdateClusterRequest) (*adminv1.UpdateClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := rejectPrimaryMutation(req.ID); err != nil {
		return nil, err
	}
	if req.Region == "" || req.EKSClusterName == "" || req.EKSClusterEndpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "region, eks_cluster_name, and eks_cluster_endpoint are required")
	}

	if err := s.clusterStore.Update(ctx, &clusterstore.Cluster{
		ID:                     req.ID,
		Region:                 req.Region,
		EKSClusterName:         req.EKSClusterName,
		EKSClusterEndpoint:     req.EKSClusterEndpoint,
		AgentIngressDomain:     req.AgentIngressDomain,
		AgentACMCertARN:        req.AgentACMCertificateARN,
		AgentALBGroupName:      req.AgentALBGroupName,
		IngestionIngressDomain: req.IngestionIngressDomain,
		IngestionACMCertARN:    req.IngestionACMCertificateARN,
		IngestionALBGroupName:  req.IngestionALBGroupName,
		KnowledgeDomain:        req.KnowledgeDomain,
		LangfuseBaseURLExt:     req.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        req.LangfuseVPCEIPs,
		PodSubnetCIDRs:         req.PodSubnetCIDRs,
	}); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}
	entry, err := s.loadAdditionalEntry(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &adminv1.UpdateClusterResponse{Cluster: entryToProto(ctx, s, entry)}, nil
}

// CheckClusterHealth evicts any cached client and re-runs the Kubernetes health check.
func (s *Server) CheckClusterHealth(ctx context.Context, req *adminv1.CheckClusterHealthRequest) (*adminv1.CheckClusterHealthResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}

	var entry k8s.ClusterEntry
	if req.ID == k8s.PrimaryClusterID {
		entries, err := s.k8sRegistry.List(ctx, false)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
		}
		if len(entries) == 0 {
			return nil, status.Error(codes.NotFound, "cluster not found")
		}
		entry = entries[0]
	} else {
		var err error
		entry, err = s.loadAdditionalEntry(ctx, req.ID)
		if err != nil {
			return nil, err
		}
	}

	return &adminv1.CheckClusterHealthResponse{Cluster: entryToProto(ctx, s, entry)}, nil
}
