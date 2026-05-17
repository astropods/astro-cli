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
	"errors"
	"fmt"
	"sync"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Errors returned by Registry.Get for additional clusters.
var (
	ErrClusterNotFound = errors.New("cluster not found")
	ErrClusterDisabled = errors.New("cluster disabled")
)

// Registry holds the ClusterClient set for the running process.
//
// The primary cluster is built once at startup from RegistryConfig and is
// what handlers and workers receive when no per-deployment cluster_id is
// recorded (i.e. `deployments.cluster_id IS NULL`). Additional clusters
// are cached by id alongside the per-deployment routing surface.
type Registry struct {
	primary      ClusterClient
	clusterStore *clusterstore.Store
	regCfg       RegistryConfig
	log          *logger.Logger

	mu    sync.RWMutex
	cache map[string]ClusterClient
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
//
// clusterStore may be nil in tests that never call Get; production passes
// the real store.
func NewRegistry(ctx context.Context, clusterStore *clusterstore.Store, cfg RegistryConfig, log *logger.Logger) (*Registry, error) {
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
	return &Registry{
		primary:      client,
		clusterStore: clusterStore,
		regCfg:       cfg,
		log:          log,
		cache:        make(map[string]ClusterClient),
	}, nil
}

// NewRegistryWithFixedPrimary returns a registry for tests or narrow call
// sites where only Default() is needed. Get returns ErrClusterNotFound for
// any non-empty id (no cluster store).
func NewRegistryWithFixedPrimary(primary ClusterClient) *Registry {
	return &Registry{
		primary: primary,
		cache:   make(map[string]ClusterClient),
	}
}

// Default returns the primary ClusterClient — the cluster astro-server is
// configured against via env vars / kubeconfig. Used by every handler and
// worker without a per-deployment cluster_id, and as the fallback for any
// deployment whose `cluster_id` is NULL.
func (r *Registry) Default() ClusterClient {
	if r == nil {
		return nil
	}
	return r.primary
}

// Get returns a ClusterClient for an additional cluster id. The id must be
// non-empty. Registered rows use EKS-style coordinates; the client is built
// with ClientModeEKS using the row's name, endpoint, and region.
func (r *Registry) Get(ctx context.Context, id string) (ClusterClient, error) {
	if r == nil {
		return nil, fmt.Errorf("registry: nil")
	}
	if id == "" {
		return nil, fmt.Errorf("registry.Get: empty cluster id")
	}

	r.mu.RLock()
	if c, ok := r.cache[id]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	if r.clusterStore == nil {
		return nil, ErrClusterNotFound
	}

	row, err := r.clusterStore.Get(ctx, id)
	if err != nil {
		if errors.Is(err, clusterstore.ErrNotFound) {
			return nil, ErrClusterNotFound
		}
		return nil, fmt.Errorf("registry.Get: %w", err)
	}
	if !row.Enabled {
		return nil, ErrClusterDisabled
	}

	c, err := NewClusterClient(ctx, ClusterClientConfig{
		Mode:            ClientModeEKS,
		ClusterName:     row.EKSClusterName,
		ClusterEndpoint: row.EKSClusterEndpoint,
		Region:          row.Region,
		Logger:          r.log,
	})
	if err != nil {
		return nil, fmt.Errorf("registry.Get: build client for %q: %w", id, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.cache[id]; ok {
		return existing, nil
	}
	r.cache[id] = c
	return c, nil
}
