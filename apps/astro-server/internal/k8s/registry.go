// Package k8s — Registry owns the ClusterClient(s) astro-server talks to.
//
// Model: one primary cluster defined by env vars / kubeconfig (the cluster
// astro-server itself is deployed into or against), plus zero or more
// additional clusters registered at runtime via the admin path (rows in
// `public.clusters`).
//
// See docs/01-spec/multi-region-cluster-support-spec.md for the design.
package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Registry holds the ClusterClient set for the running process.
//
// The primary cluster is built once at startup from RegistryConfig and is
// what handlers and workers receive when no per-deployment cluster_id is
// recorded (i.e. `deployments.cluster_id IS NULL`). Additional clusters
// are cached by id alongside the per-deployment routing surface.
type Registry struct {
	primary ClusterClient
}

// RegistryConfig is the process-level Kubernetes configuration that the
// primary ClusterClient needs. EKS and Local fields are populated from the
// `cfg.Deployment.*` env-var-backed fields by `main.go`.
type RegistryConfig struct {
	Mode             ClientMode
	Region           string
	KubeconfigPath   string
	KubeContext      string
	EKSBootstrapName string
	EKSBootstrapURL  string
}

// NewRegistry builds the registry's primary ClusterClient from cfg. Returns
// an error if client construction fails so astro-server fails to boot
// rather than running without a routable cluster.
func NewRegistry(ctx context.Context, cfg RegistryConfig, log *logger.Logger) (*Registry, error) {
	client, err := NewClusterClient(ctx, ClusterClientConfig{
		Mode:            cfg.Mode,
		ClusterName:     cfg.EKSBootstrapName,
		ClusterEndpoint: cfg.EKSBootstrapURL,
		Region:          cfg.Region,
		KubeconfigPath:  cfg.KubeconfigPath,
		KubeContext:     cfg.KubeContext,
		Logger:          log,
	})
	if err != nil {
		return nil, fmt.Errorf("registry: build primary cluster client: %w", err)
	}
	return &Registry{primary: client}, nil
}

// Default returns the primary ClusterClient — the cluster astro-server is
// configured against via env vars / kubeconfig. Used by every handler and
// worker without a per-deployment cluster_id, and as the fallback for any
// deployment whose `cluster_id` is NULL.
func (r *Registry) Default() ClusterClient {
	return r.primary
}
