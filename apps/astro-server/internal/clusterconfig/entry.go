package clusterconfig

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
)

type Entry struct {
	ID                       string `json:"id"`
	Region                   string `json:"region"`
	EKSClusterName           string `json:"eks_cluster_name"`
	EKSClusterEndpoint       string `json:"eks_cluster_endpoint"`
	EKSClusterCA             string `json:"eks_cluster_ca"`
	AgentIngressDomain       string `json:"agent_ingress_domain"`
	AgentPublicIngressDomain string `json:"agent_public_ingress_domain"`
	IngestionIngressDomain   string `json:"ingestion_ingress_domain"`
	LangfuseBaseURLExt       string `json:"langfuse_base_url_ext"`
	LangfuseVPCEIPs          string `json:"langfuse_vpce_ips"`
	PodSubnetCIDRs           string `json:"pod_subnet_cidrs"`
	PodSubnetIPv6CIDRs       string `json:"pod_subnet_ipv6_cidrs"`
	LokiURL                  string `json:"loki_url"`
	PrometheusURL            string `json:"prometheus_url"`
	TenantRouterInternalURL  string `json:"tenant_router_internal_url"`
}

func Load(path string) ([]Entry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read cluster config %q: %w", path, err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse cluster config %q: %w", path, err)
	}
	return entries, nil
}

func Find(entries []Entry, id string) (Entry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

func (e Entry) DecodedCA() ([]byte, error) {
	ca, err := base64.StdEncoding.DecodeString(e.EKSClusterCA)
	if err != nil {
		return nil, fmt.Errorf("decode eks_cluster_ca for %q: %w", e.ID, err)
	}
	return ca, nil
}

func (e Entry) ToClusterRow() (*clusterstore.Cluster, error) {
	ca, err := e.DecodedCA()
	if err != nil {
		return nil, err
	}
	return &clusterstore.Cluster{
		ID:                       e.ID,
		Region:                   e.Region,
		EKSClusterName:           e.EKSClusterName,
		EKSClusterEndpoint:       e.EKSClusterEndpoint,
		EKSClusterCA:             ca,
		AgentIngressDomain:       e.AgentIngressDomain,
		AgentPublicIngressDomain: e.AgentPublicIngressDomain,
		IngestionIngressDomain:   e.IngestionIngressDomain,
		LangfuseBaseURLExt:       e.LangfuseBaseURLExt,
		LangfuseVPCEIPs:          e.LangfuseVPCEIPs,
		PodSubnetCIDRs:           e.PodSubnetCIDRs,
		PodSubnetIPv6CIDRs:       e.PodSubnetIPv6CIDRs,
		LokiURL:                  e.LokiURL,
		PrometheusURL:            e.PrometheusURL,
		TenantRouterInternalURL:  e.TenantRouterInternalURL,
	}, nil
}
