package clusterfields

import (
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/commalist"
)

type namedValue struct {
	name  string
	value string
}

// deployFields returns ordered (column name, value) pairs for deploy config.
// Order is stable so tests and error messages stay predictable.
// langfuse_vpce_ips is deliberately absent: it's optional on every cluster —
// only clusters that need a PrivateLink netpol exception to reach Langfuse
// (cross-region ones) set it at all.
func deployFields(d DeployConfig) []namedValue {
	return []namedValue{
		{"agent_ingress_domain", d.AgentIngressDomain},
		{"ingestion_ingress_domain", d.IngestionIngressDomain},
		{"langfuse_base_url_ext", d.LangfuseBaseURLExt},
		{"pod_subnet_cidrs", d.PodSubnetCIDRs},
	}
}

func registrationFields(r Registration) []namedValue {
	return append([]namedValue{
		{"region", r.Region},
		{"eks_cluster_name", r.EKSClusterName},
		{"eks_cluster_endpoint", r.EKSClusterEndpoint},
	}, deployFields(r.Deploy)...)
}

func presentError(clusterID, fieldName string) error {
	if clusterID != "" {
		return fmt.Errorf("cluster %q is missing required field %s", clusterID, fieldName)
	}
	return fmt.Errorf("%s is required", fieldName)
}

func checkPresent(clusterID string, fields []namedValue) error {
	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			return presentError(clusterID, f.name)
		}
	}
	return nil
}

func checkCommaListsPresent(clusterID string, d DeployConfig) error {
	if len(commalist.Parse(d.PodSubnetCIDRs)) == 0 {
		return presentError(clusterID, "pod_subnet_cidrs")
	}
	return nil
}

// ValidateDeployNonEmpty ensures every deploy config field is set. Pass an
// empty clusterID for store-style errors ("%s is required"); pass the cluster
// id for deploy-time errors ("cluster %q is missing required field %s").
func ValidateDeployNonEmpty(clusterID string, d DeployConfig) error {
	if err := checkPresent(clusterID, deployFields(d)); err != nil {
		return err
	}
	return checkCommaListsPresent(clusterID, d)
}

// ValidateRegistrationNonEmpty checks EKS fields plus deploy config. Used by
// clusterstore.UpsertFromConfig; clusterID is always empty.
func ValidateRegistrationNonEmpty(r Registration) error {
	if err := checkPresent("", registrationFields(r)); err != nil {
		return err
	}
	return checkCommaListsPresent("", r.Deploy)
}
