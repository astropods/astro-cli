package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/gin-gonic/gin"
)

func k8sRegistryReady(reg *k8s.Registry) bool {
	return reg != nil && reg.Default() != nil
}

// deploymentClusterClient returns the ClusterClient for a deployment row.
func deploymentClusterClient(ctx context.Context, reg *k8s.Registry, dep *deploymentstore.Deployment) (k8s.ClusterClient, error) {
	if reg == nil {
		return nil, fmt.Errorf("kubernetes registry not configured")
	}
	if dep == nil {
		return nil, fmt.Errorf("nil deployment")
	}
	if dep.EffectiveClusterID() == "" {
		if reg.Default() == nil {
			return nil, fmt.Errorf("kubernetes client not configured")
		}
		return reg.Default(), nil
	}
	return reg.Get(ctx, dep.EffectiveClusterID())
}

// clusterHealthForDeploy resolves the target cluster client and runs
// CheckHealth before accepting a deploy that sets target.cluster_id.
func clusterHealthForDeploy(ctx context.Context, reg *k8s.Registry, clusterID string) error {
	if !k8sRegistryReady(reg) {
		return fmt.Errorf("kubernetes client not configured")
	}
	kc, err := reg.Get(ctx, clusterID)
	if err != nil {
		return err
	}
	return kc.CheckHealth()
}

// clusterClientForDeployment resolves the K8s client for dep and writes an HTTP
// error to c when resolution fails. The bool is false when the handler should return.
func clusterClientForDeployment(c *gin.Context, reg *k8s.Registry, dep *deploymentstore.Deployment) (k8s.ClusterClient, bool) {
	if !k8sRegistryReady(reg) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
		return nil, false
	}
	kc, err := deploymentClusterClient(c.Request.Context(), reg, dep)
	if err != nil {
		switch {
		case errors.Is(err, k8s.ErrClusterNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		case errors.Is(err, k8s.ErrClusterDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster disabled"})
		default:
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not configured"})
		}
		return nil, false
	}
	return kc, true
}
