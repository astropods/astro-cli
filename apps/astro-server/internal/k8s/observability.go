package k8s

import (
	"context"
	"fmt"
)

// PrometheusClusterFilter returns a PromQL label selector fragment for the
// deployment's target cluster (e.g. `,cluster="preview-eu-managed-eks"`).
// deploymentClusterID is deployments.cluster_id (empty = primary).
func (r *Registry) PrometheusClusterFilter(ctx context.Context, deploymentClusterID string) string {
	if r == nil {
		return ""
	}
	entry, err := r.GetEntry(ctx, deploymentClusterID)
	if err != nil || entry.EKSClusterName == "" {
		return ""
	}
	return fmt.Sprintf(`,cluster="%s"`, entry.EKSClusterName)
}

// LokiClusterName returns the Alloy `cluster` stream label for a deployment.
func (r *Registry) LokiClusterName(ctx context.Context, deploymentClusterID string) string {
	if r == nil {
		return ""
	}
	entry, err := r.GetEntry(ctx, deploymentClusterID)
	if err != nil || entry.EKSClusterName == "" {
		return ""
	}
	return entry.EKSClusterName
}
