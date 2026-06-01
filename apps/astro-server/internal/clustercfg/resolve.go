// Package clustercfg returns the effective ingress / ALB / cert / knowledge
// and Langfuse / netpol configuration for one deployment. The primary cluster
// reads env defaults (cfg.Deployment.*); additional clusters read their row
// from public.clusters verbatim. Env defaults never apply to a non-primary
// cluster — those rows must declare a complete config.
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
	AgentIngressDomain     string
	AgentACMCertARN        string
	AgentALBGroupName      string
	IngestionIngressDomain string
	IngestionACMCertARN    string
	IngestionALBGroupName  string
	KnowledgeDomain        string
	LangfuseBaseURL        string   // collector LANGFUSE_BASE_URL
	LangfuseVPCEIPs        []string // VPCE ENI IPs for netpol :3000 egress
	PodSubnetCIDRs         []string // pod subnet CIDRs for netpol except list
}

// Resolve returns the effective config for a deployment targeting clusterID.
//
// clusterID == "" (or k8s.PrimaryClusterID) returns env defaults from dep.
// For an additional cluster the row's values are returned verbatim with no
// env fallback; any empty required field is a configuration error.
func Resolve(ctx context.Context, reg *k8s.Registry, dep config.DeploymentConfig, clusterID string) (Resolved, error) {
	if clusterID == "" || clusterID == k8s.PrimaryClusterID || reg == nil {
		langfuseURL := dep.LangfuseBaseURLExt
		if langfuseURL == "" {
			langfuseURL = dep.LangfuseBaseURL
		}
		return Resolved{
			AgentIngressDomain:     dep.IngressDomain,
			AgentACMCertARN:        dep.ACMCertificateARN,
			AgentALBGroupName:      dep.ALBGroupName,
			IngestionIngressDomain: dep.IngestionIngressDomain,
			IngestionACMCertARN:    dep.IngestionACMCertARN,
			IngestionALBGroupName:  dep.IngestionALBGroupName,
			KnowledgeDomain:        dep.KnowledgeDomain,
			LangfuseBaseURL:        langfuseURL,
			LangfuseVPCEIPs:        dep.LangfuseVPCEIPs,
			PodSubnetCIDRs:         dep.PodSubnetCIDRs,
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

	return Resolved{
		AgentIngressDomain:     entry.AgentIngressDomain,
		AgentACMCertARN:        entry.AgentACMCertARN,
		AgentALBGroupName:      entry.AgentALBGroupName,
		IngestionIngressDomain: entry.IngestionIngressDomain,
		IngestionACMCertARN:    entry.IngestionACMCertARN,
		IngestionALBGroupName:  entry.IngestionALBGroupName,
		KnowledgeDomain:        entry.KnowledgeDomain,
		LangfuseBaseURL:        entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        commalist.Parse(entry.LangfuseVPCEIPs),
		PodSubnetCIDRs:         commalist.Parse(entry.PodSubnetCIDRs),
	}, nil
}

func deployConfigFromEntry(entry k8s.ClusterEntry) clusterfields.DeployConfig {
	return clusterfields.DeployConfig{
		AgentIngressDomain:     entry.AgentIngressDomain,
		AgentACMCertARN:        entry.AgentACMCertARN,
		AgentALBGroupName:      entry.AgentALBGroupName,
		IngestionIngressDomain: entry.IngestionIngressDomain,
		IngestionACMCertARN:    entry.IngestionACMCertARN,
		IngestionALBGroupName:  entry.IngestionALBGroupName,
		KnowledgeDomain:        entry.KnowledgeDomain,
		LangfuseBaseURLExt:     entry.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        entry.LangfuseVPCEIPs,
		PodSubnetCIDRs:         entry.PodSubnetCIDRs,
	}
}
