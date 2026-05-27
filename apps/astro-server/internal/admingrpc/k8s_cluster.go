package admingrpc

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

func (s *Server) registryReady() bool {
	return s.k8sRegistry != nil && s.k8sRegistry.Default() != nil
}

// deploymentClusterClient resolves the Kubernetes client for a deployment row.
// nil dep routes to the primary cluster.
func (s *Server) deploymentClusterClient(ctx context.Context, dep *deploymentstore.Deployment) (k8s.ClusterClient, error) {
	if s.registryReady() {
		if dep == nil || dep.EffectiveClusterID() == "" {
			return s.k8sRegistry.Default(), nil
		}
		return s.k8sRegistry.Get(ctx, dep.EffectiveClusterID())
	}
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}
	return s.k8sClient, nil
}

// clusterClientForNamespace resolves the client for a tenant namespace by looking
// up the deployment row. Unknown namespaces fall back to the primary cluster.
func (s *Server) clusterClientForNamespace(ctx context.Context, namespace string) (k8s.ClusterClient, error) {
	if namespace == "" {
		return s.deploymentClusterClient(ctx, nil)
	}
	dep, err := s.deployStore.GetDeploymentByNamespace(namespace)
	if err != nil {
		return nil, fmt.Errorf("get deployment by namespace: %w", err)
	}
	return s.deploymentClusterClient(ctx, dep)
}
