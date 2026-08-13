package admingrpc

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// urlReachabilityTimeout bounds each TCP dial so a single unreachable URL
// can't stall the health-check button.
const urlReachabilityTimeout = 3 * time.Second

// checkURLReachability dials the host:port for a URL (or bare host:port
// value, for fields like tenant_router_internal_url that don't carry a
// scheme) and reports whether a TCP connection succeeds. It doesn't issue an
// HTTP request, since several of these endpoints require auth and would
// otherwise report false negatives on 401/403.
func checkURLReachability(ctx context.Context, rawURL string) (reachable bool, errMsg string) {
	hostport := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		hostport = u.Host
	}
	if _, _, err := net.SplitHostPort(hostport); err != nil {
		if strings.HasPrefix(rawURL, "https://") {
			hostport = net.JoinHostPort(hostport, "443")
		} else {
			hostport = net.JoinHostPort(hostport, "80")
		}
	}

	dialCtx, cancel := context.WithTimeout(ctx, urlReachabilityTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", hostport)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
}

// checkClusterURLs runs checkURLReachability concurrently for every
// non-empty optional observability/netpol URL on the cluster.
func checkClusterURLs(ctx context.Context, entry k8s.ClusterEntry) []adminv1.UrlReachability {
	candidates := []struct{ label, url string }{
		{"langfuse_base_url_ext", entry.LangfuseBaseURLExt},
		{"loki_url", entry.LokiURL},
		{"prometheus_url", entry.PrometheusURL},
		{"tenant_router_internal_url", entry.TenantRouterInternalURL},
	}

	slots := make([]*adminv1.UrlReachability, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		if c.url == "" {
			continue
		}
		wg.Add(1)
		go func(i int, label, rawURL string) {
			defer wg.Done()
			reachable, errMsg := checkURLReachability(ctx, rawURL)
			slots[i] = &adminv1.UrlReachability{
				Label:     label,
				URL:       rawURL,
				Reachable: reachable,
				Error:     errMsg,
			}
		}(i, c.label, c.url)
	}
	wg.Wait()

	results := make([]adminv1.UrlReachability, 0, len(candidates))
	for _, s := range slots {
		if s != nil {
			results = append(results, *s)
		}
	}
	return results
}

func (s *Server) requireClusterAdmin() error {
	if s.clusterStore == nil || s.k8sRegistry == nil {
		return status.Error(codes.FailedPrecondition, "kubernetes registry not configured")
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
	if errors.Is(err, clusterstore.ErrInUse) ||
		errors.Is(err, clusterstore.ErrInUseByAccounts) ||
		errors.Is(err, clusterstore.ErrInUseByDeployments) {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Error(codes.InvalidArgument, err.Error())
}

func (s *Server) clusterHealth(ctx context.Context, entry k8s.ClusterEntry) (healthy bool, healthErr string) {
	var client k8s.ClusterClient
	if entry.IsDefault {
		client = s.k8sRegistry.Default()
	} else {
		var err error
		client, err = s.k8sRegistry.Get(ctx, entry.ID)
		if err != nil {
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
		ID:                      entry.ID,
		Region:                  entry.Region,
		EKSClusterName:          entry.EKSClusterName,
		EKSClusterEndpoint:      entry.EKSClusterEndpoint,
		EKSClusterCA:            entry.EKSClusterCA,
		IsPrimary:               entry.IsDefault,
		Healthy:                 healthy,
		HealthError:             healthErr,
		AgentIngressDomain:      entry.AgentIngressDomain,
		IngestionIngressDomain:  entry.IngestionIngressDomain,
		LangfuseBaseURLExt:      entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:         entry.LangfuseVPCEIPs,
		PodSubnetCIDRs:          entry.PodSubnetCIDRs,
		PodSubnetIPv6CIDRs:      entry.PodSubnetIPv6CIDRs,
		LokiURL:                 entry.LokiURL,
		PrometheusURL:           entry.PrometheusURL,
		TenantRouterInternalURL: entry.TenantRouterInternalURL,
		CreatedAt:               entry.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:               entry.UpdatedAt.UTC().Format(time.RFC3339),
	}
	return out
}

func (s *Server) loadEntry(ctx context.Context, id string) (k8s.ClusterEntry, error) {
	entry, err := s.k8sRegistry.GetEntry(ctx, id)
	if err != nil {
		if errors.Is(err, k8s.ErrClusterNotFound) {
			return k8s.ClusterEntry{}, status.Error(codes.NotFound, "cluster not found")
		}
		return k8s.ClusterEntry{}, status.Error(codes.Internal, err.Error())
	}
	return entry, nil
}

// DeregisterCluster deletes an additional cluster row when no deployments reference it.
func (s *Server) DeregisterCluster(ctx context.Context, req *adminv1.DeregisterClusterRequest) (*adminv1.DeregisterClusterResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.clusterStore.Deregister(ctx, req.ID); err != nil {
		return nil, clusterStoreErr(err)
	}
	if err := s.k8sRegistry.Refresh(ctx, req.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "refresh registry: %v", err)
	}
	return &adminv1.DeregisterClusterResponse{}, nil
}

// GetClusterBlockers lists the accounts and deployments that currently
// reference a cluster and would fail DeregisterCluster's FK-backed delete.
func (s *Server) GetClusterBlockers(ctx context.Context, req *adminv1.GetClusterBlockersRequest) (*adminv1.GetClusterBlockersResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	accounts, accountCount, deployments, deploymentCount, err := s.clusterStore.Blockers(ctx, req.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list cluster blockers: %v", err)
	}

	return &adminv1.GetClusterBlockersResponse{
		AccountCount:    int32(accountCount), //nolint:gosec // bounded by account count, not attacker-controlled
		Accounts:        blockersToProto(accounts),
		DeploymentCount: int32(deploymentCount), //nolint:gosec // bounded by deployment count, not attacker-controlled
		Deployments:     blockersToProto(deployments),
	}, nil
}

func blockersToProto(blockers []clusterstore.Blocker) []*adminv1.ClusterBlocker {
	out := make([]*adminv1.ClusterBlocker, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, &adminv1.ClusterBlocker{ID: b.ID, Name: b.Name, Status: b.Status})
	}
	return out
}

// ListClusters returns every cluster row.
func (s *Server) ListClusters(ctx context.Context, req *adminv1.ListClustersRequest) (*adminv1.ListClustersResponse, error) {
	if err := s.requireClusterAdmin(); err != nil {
		return nil, err
	}
	entries, err := s.k8sRegistry.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
	}
	out := make([]*adminv1.RegisteredCluster, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entryToProto(ctx, s, entry))
	}
	return &adminv1.ListClustersResponse{Clusters: out}, nil
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

	entry, err := s.loadEntry(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &adminv1.CheckClusterHealthResponse{
		Cluster:   entryToProto(ctx, s, entry),
		UrlChecks: checkClusterURLs(ctx, entry),
	}, nil
}
