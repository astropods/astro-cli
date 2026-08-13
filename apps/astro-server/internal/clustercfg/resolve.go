// Package clustercfg returns the effective ingress / knowledge / Langfuse /
// netpol configuration for one deployment, from the cluster's row in
// public.clusters.
package clustercfg

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/clusterfields"
	"github.com/astropods/astro/apps/astro-server/internal/commalist"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// Resolved is the effective per-deployment configuration. Ingress field names
// mirror public.clusters column names; observability fields drive collector
// Langfuse export and tenant NetworkPolicy egress.
type Resolved struct {
	AgentIngressDomain       string
	AgentPublicIngressDomain string
	IngestionIngressDomain   string
	LangfuseBaseURL          string   // collector LANGFUSE_BASE_URL
	LangfuseVPCEIPs          []string // VPCE ENI IPs for netpol :3000 egress
	PodSubnetCIDRs           []string // pod subnet CIDRs for netpol except list
	PodSubnetIPv6CIDRs       []string // IPv6 counterpart to PodSubnetCIDRs; empty for IPv4-only clusters
	CPSubnetCIDRs            []string // apiserver ENI subnets for service-proxy ingress NP
	RegistryPullCredential   string   // CPC embedded in this cluster's tenant image-pull Secret
}

// Resolve returns the effective config for a deployment targeting clusterID.
// clusterID == "" resolves to the default cluster. CPSubnetCIDRs has no
// per-cluster equivalent yet, so it always comes from dep and only applies
// to the default cluster.
//
// When no default cluster is configured (DEFAULT_CLUSTER_ID unset — local
// dev, where there's no cluster-config to sync), clusterID == "" falls back
// to dep verbatim instead of validating against the registry, same as the
// reg == nil case: there's no row to resolve against.
func Resolve(ctx context.Context, reg *k8s.Registry, dep config.DeploymentConfig, clusterID string) (Resolved, error) {
	if reg == nil || (clusterID == "" && reg.DefaultClusterID() == "") {
		langfuseURL := dep.LangfuseBaseURLExt
		if langfuseURL == "" {
			langfuseURL = dep.LangfuseBaseURL
		}
		return Resolved{
			AgentIngressDomain:       dep.IngressDomain,
			AgentPublicIngressDomain: dep.AgentPublicIngressDomain,
			IngestionIngressDomain:   dep.IngestionIngressDomain,
			LangfuseBaseURL:          langfuseURL,
			LangfuseVPCEIPs:          dep.LangfuseVPCEIPs,
			PodSubnetCIDRs:           dep.PodSubnetCIDRs,
			PodSubnetIPv6CIDRs:       dep.PodSubnetIPv6CIDRs,
			CPSubnetCIDRs:            dep.CPSubnetCIDRs,
			RegistryPullCredential:   dep.RegistryPullCredential,
		}, nil
	}

	entry, err := reg.GetEntry(ctx, clusterID)
	if err != nil {
		if errors.Is(err, k8s.ErrClusterNotFound) {
			return Resolved{}, fmt.Errorf("cluster %q not found", clusterID)
		}
		return Resolved{}, fmt.Errorf("resolve cluster %q: %w", clusterID, err)
	}

	if err := clusterfields.ValidateDeployNonEmpty(clusterID, deployConfigFromEntry(entry)); err != nil {
		return Resolved{}, err
	}
	if dep.ProxyRegistryHost != "" && entry.PullCredential == "" {
		return Resolved{}, fmt.Errorf("cluster %q has no registry pull credential — register or update the cluster to generate one", clusterID)
	}

	resolved := Resolved{
		AgentIngressDomain:       entry.AgentIngressDomain,
		AgentPublicIngressDomain: entry.AgentPublicIngressDomain,
		IngestionIngressDomain:   entry.IngestionIngressDomain,
		LangfuseBaseURL:          entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:          commalist.Parse(entry.LangfuseVPCEIPs),
		PodSubnetCIDRs:           commalist.Parse(entry.PodSubnetCIDRs),
		PodSubnetIPv6CIDRs:       commalist.Parse(entry.PodSubnetIPv6CIDRs),
		RegistryPullCredential:   entry.PullCredential,
	}
	if entry.IsDefault {
		if resolved.LangfuseBaseURL == "" {
			resolved.LangfuseBaseURL = dep.LangfuseBaseURL
		}
		resolved.CPSubnetCIDRs = dep.CPSubnetCIDRs
	}
	return resolved, nil
}

func deployConfigFromEntry(entry k8s.ClusterEntry) clusterfields.DeployConfig {
	return clusterfields.DeployConfig{
		AgentIngressDomain:     entry.AgentIngressDomain,
		IngestionIngressDomain: entry.IngestionIngressDomain,
		LangfuseBaseURLExt:     entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        entry.LangfuseVPCEIPs,
		PodSubnetCIDRs:         entry.PodSubnetCIDRs,
	}
}
