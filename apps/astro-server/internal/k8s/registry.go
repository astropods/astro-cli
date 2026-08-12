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
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// PrimaryClusterID is the stable identifier for the env-var-defined primary
// cluster in admin List/health responses. It is not a clusters table row.
const PrimaryClusterID = "primary"

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

	mu         sync.RWMutex
	cache      map[string]ClusterClient
	entryCache map[string]ClusterEntry
	lokiCache  map[string]*loki.Client
	promCache  map[string]*promquery.Client
}

// ClusterEntry is a registry-level view of one cluster (primary or additional).
//
// For additional clusters the ingress/cert/knowledge fields are the raw
// values stored in the clusters table — every field is required at write
// time (see clusterstore.validateRequiredFields) and clustercfg.Resolve
// rejects empties at deploy time. For the primary cluster these fields
// are left empty here; callers must read the env-derived values directly
// from cfg.Deployment.* (see clustercfg.Resolve, which handles the primary
// vs. additional split).
type ClusterEntry struct {
	ID                     string
	IsPrimary              bool
	Region                 string
	EKSClusterName         string
	EKSClusterEndpoint     string
	EKSClusterCA           []byte // PEM CA bytes, empty for primary
	Enabled                bool
	AgentIngressDomain     string
	IngestionIngressDomain string
	LangfuseBaseURLExt     string
	LangfuseVPCEIPs        string
	PodSubnetCIDRs         string
	PodSubnetIPv6CIDRs     string
	// LokiURL and PrometheusURL are optional per-cluster query endpoint
	// overrides. Empty for both the primary cluster and any additional
	// cluster without its own observability stack — callers fall back to the
	// global LOKI_URL/PROMETHEUS_URL client in that case (see LokiClientFor /
	// PrometheusClientFor in observability.go).
	LokiURL       string
	PrometheusURL string
	// PullCredential is never exposed via the admin API; only clustercfg.Resolve reads it.
	PullCredential string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ClusterEntryFromRow maps a clusterstore row onto the registry's ClusterEntry
// view. The only conversion site for additional (non-primary) clusters —
// GetEntry, List, and admingrpc's rowToEntry all call this — so a field added
// to either struct only needs wiring in one place instead of three drifting
// copies (see the pod_subnet_ipv6_cidrs incident this replaced).
func ClusterEntryFromRow(row *clusterstore.Cluster) ClusterEntry {
	return ClusterEntry{
		ID:                     row.ID,
		IsPrimary:              false,
		Region:                 row.Region,
		EKSClusterName:         row.EKSClusterName,
		EKSClusterEndpoint:     row.EKSClusterEndpoint,
		EKSClusterCA:           row.EKSClusterCA,
		Enabled:                row.Enabled,
		AgentIngressDomain:     row.AgentIngressDomain,
		IngestionIngressDomain: row.IngestionIngressDomain,
		LangfuseBaseURLExt:     row.LangfuseBaseURLExt,
		LangfuseVPCEIPs:        row.LangfuseVPCEIPs,
		PodSubnetCIDRs:         row.PodSubnetCIDRs,
		PodSubnetIPv6CIDRs:     row.PodSubnetIPv6CIDRs,
		LokiURL:                row.LokiURL,
		PrometheusURL:          row.PrometheusURL,
		PullCredential:         row.PullCredential,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
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
		entryCache:   make(map[string]ClusterEntry),
	}, nil
}

// NewRegistryWithPrimary returns a registry whose Default() is the given client.
// No clusterstore is attached — Get(id) returns ErrClusterNotFound for every id.
// Use NewRegistry to build the primary from env/config, or NewRegistryWithClusterStore
// when tests need additional-cluster lookup.
func NewRegistryWithPrimary(primary ClusterClient) *Registry {
	return &Registry{
		primary: primary,
		cache:   make(map[string]ClusterClient),
	}
}

// NewRegistryWithClusterStore returns a registry for tests that need both
// Default() and Get() backed by clusterstore rows.
func NewRegistryWithClusterStore(primary ClusterClient, clusterStore *clusterstore.Store, log *logger.Logger) *Registry {
	if log == nil {
		log = logger.New("error", "json")
	}
	return &Registry{
		primary:      primary,
		clusterStore: clusterStore,
		log:          log,
		cache:        make(map[string]ClusterClient),
	}
}

// NewRegistryForTest wires clusterstore and RegistryConfig for tests that call List().
func NewRegistryForTest(primary ClusterClient, clusterStore *clusterstore.Store, cfg RegistryConfig) *Registry {
	return &Registry{
		primary:      primary,
		clusterStore: clusterStore,
		regCfg:       cfg,
		log:          logger.New("error", "json"),
		cache:        make(map[string]ClusterClient),
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
		ClusterCA:       row.EKSClusterCA,
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

// GetEntry returns the ClusterEntry for an id. An empty id resolves to the
// primary entry; any other id reads from clusterstore. Returns ErrClusterNotFound
// when the additional row is missing.
//
// Additional-cluster entries are cached so the deploy path (which both
// validates the cluster and resolves its ingress config) doesn't re-query
// for every step. Refresh evicts cached entries; the cluster admin RPCs
// already call Refresh after every mutation.
func (r *Registry) GetEntry(ctx context.Context, id string) (ClusterEntry, error) {
	if r == nil {
		return ClusterEntry{}, fmt.Errorf("registry: nil")
	}
	if id == "" || id == PrimaryClusterID {
		return r.primaryEntry(), nil
	}

	r.mu.RLock()
	if entry, ok := r.entryCache[id]; ok {
		r.mu.RUnlock()
		return entry, nil
	}
	r.mu.RUnlock()

	if r.clusterStore == nil {
		return ClusterEntry{}, ErrClusterNotFound
	}
	row, err := r.clusterStore.Get(ctx, id)
	if err != nil {
		if errors.Is(err, clusterstore.ErrNotFound) {
			return ClusterEntry{}, ErrClusterNotFound
		}
		return ClusterEntry{}, fmt.Errorf("registry.GetEntry: %w", err)
	}
	entry := ClusterEntryFromRow(row)
	r.mu.Lock()
	if r.entryCache == nil {
		r.entryCache = make(map[string]ClusterEntry)
	}
	r.entryCache[id] = entry
	r.mu.Unlock()
	return entry, nil
}

func (r *Registry) primaryEntry() ClusterEntry {
	return ClusterEntry{
		ID:                 PrimaryClusterID,
		IsPrimary:          true,
		Region:             r.regCfg.Region,
		EKSClusterName:     r.regCfg.EKSBootstrapName,
		EKSClusterEndpoint: r.regCfg.EKSBootstrapURL,
		Enabled:            true,
	}
}

// List returns the primary cluster (synthesized) plus additional clusters from
// clusterstore. When enabledOnly is true, disabled additional rows are omitted;
// the primary is always included and treated as enabled.
func (r *Registry) List(ctx context.Context, enabledOnly bool) ([]ClusterEntry, error) {
	if r == nil {
		return nil, fmt.Errorf("registry: nil")
	}

	out := []ClusterEntry{r.primaryEntry()}

	if r.clusterStore == nil {
		return out, nil
	}

	rows, err := r.clusterStore.List(ctx, enabledOnly)
	if err != nil {
		return nil, fmt.Errorf("registry.List: %w", err)
	}
	for _, row := range rows {
		out = append(out, ClusterEntryFromRow(row))
	}
	return out, nil
}

// SetCachedEntryForTest pre-seeds GetEntry(id) so tests don't need a sqlmock.
func (r *Registry) SetCachedEntryForTest(id string, entry ClusterEntry) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entryCache == nil {
		r.entryCache = make(map[string]ClusterEntry)
	}
	r.entryCache[id] = entry
}

// SetCachedClientForTest pre-seeds Get(id) for tests that must avoid dialing EKS.
func (r *Registry) SetCachedClientForTest(id string, c ClusterClient) {
	if r == nil || id == "" || c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = make(map[string]ClusterClient)
	}
	r.cache[id] = c
}

// Refresh drops a cached additional-cluster client so the next Get re-reads
// the row. No-op for the primary cluster id.
func (r *Registry) Refresh(_ context.Context, id string) error {
	if r == nil {
		return fmt.Errorf("registry: nil")
	}
	if id == "" || id == PrimaryClusterID {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, id)
	delete(r.entryCache, id)
	delete(r.lokiCache, id)
	delete(r.promCache, id)
	return nil
}
