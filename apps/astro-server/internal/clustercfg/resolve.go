// Package clustercfg returns the effective ingress / ALB / cert / knowledge
// configuration for one deployment. The primary cluster reads env defaults
// (cfg.Deployment.*); additional clusters read their row from public.clusters
// verbatim. Env defaults never apply to a non-primary cluster — those rows
// must declare a complete config (see clusterstore.validateIngressFields).
package clustercfg

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// Resolved is the effective per-deployment ingress configuration. Field names
// mirror the public.clusters column names (agent_* for the agent ALB, ingestion_*
// for the ingestion ALB) so the wire / table / Go layers stay consistent.
type Resolved struct {
	AgentIngressDomain     string
	AgentACMCertARN        string
	AgentALBGroupName      string
	IngestionIngressDomain string
	IngestionACMCertARN    string
	IngestionALBGroupName  string
	KnowledgeDomain        string
}

// Resolve returns the effective config for a deployment targeting clusterID.
//
// clusterID == "" (or k8s.PrimaryClusterID) returns the env defaults from dep.
// For an additional cluster the row's values are returned verbatim with no
// env fallback; any empty field is a configuration error.
//
// Errors:
//   - cluster not found in the registry
//   - non-primary cluster with an empty required ingress field
func Resolve(ctx context.Context, reg *k8s.Registry, dep config.DeploymentConfig, clusterID string) (Resolved, error) {
	if clusterID == "" || clusterID == k8s.PrimaryClusterID || reg == nil {
		return Resolved{
			AgentIngressDomain:     dep.IngressDomain,
			AgentACMCertARN:        dep.ACMCertificateARN,
			AgentALBGroupName:      dep.ALBGroupName,
			IngestionIngressDomain: dep.IngestionIngressDomain,
			IngestionACMCertARN:    dep.IngestionACMCertARN,
			IngestionALBGroupName:  dep.IngestionALBGroupName,
			KnowledgeDomain:        dep.KnowledgeDomain,
		}, nil
	}

	entry, err := reg.GetEntry(ctx, clusterID)
	if err != nil {
		if errors.Is(err, k8s.ErrClusterNotFound) {
			return Resolved{}, fmt.Errorf("cluster %q not found", clusterID)
		}
		return Resolved{}, fmt.Errorf("resolve cluster %q: %w", clusterID, err)
	}

	if err := requireNonEmpty(clusterID, entry); err != nil {
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
	}, nil
}

// requireNonEmpty mirrors clusterstore.validateIngressFields. The store
// validates at write time, but pre-existing rows from before these columns
// were added carry empties; this catches them at deploy time rather than
// emitting broken ALB annotations.
func requireNonEmpty(clusterID string, entry k8s.ClusterEntry) error {
	fields := []struct {
		name  string
		value string
	}{
		{"agent_ingress_domain", entry.AgentIngressDomain},
		{"agent_acm_certificate_arn", entry.AgentACMCertARN},
		{"agent_alb_group_name", entry.AgentALBGroupName},
		{"ingestion_ingress_domain", entry.IngestionIngressDomain},
		{"ingestion_acm_certificate_arn", entry.IngestionACMCertARN},
		{"ingestion_alb_group_name", entry.IngestionALBGroupName},
		{"knowledge_domain", entry.KnowledgeDomain},
	}
	for _, f := range fields {
		if f.value == "" {
			return fmt.Errorf("cluster %q is missing required ingress field %s", clusterID, f.name)
		}
	}
	return nil
}
