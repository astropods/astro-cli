// Package k8s — Registry owns the ClusterClient(s) astro-server talks to.
//
// Every managed cluster is a row in `public.clusters`, synced at boot from
// astro-infra's cluster config. DefaultClusterID names the row astro-server
// itself runs on.
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

var ErrClusterNotFound = errors.New("cluster not found")

type Registry struct {
	primary          ClusterClient
	defaultClusterID string
	clusterStore     *clusterstore.Store
	log              *logger.Logger

	mu         sync.RWMutex
	cache      map[string]ClusterClient
	entryCache map[string]ClusterEntry
	lokiCache  map[string]*loki.Client
	promCache  map[string]*promquery.Client
}

type ClusterEntry struct {
	ID                       string
	IsDefault                bool
	Region                   string
	EKSClusterName           string
	EKSClusterEndpoint       string
	EKSClusterCA             []byte
	AgentIngressDomain       string
	AgentPublicIngressDomain string
	IngestionIngressDomain   string
	LangfuseBaseURLExt       string
	LangfuseVPCEIPs          string
	PodSubnetCIDRs           string
	PodSubnetIPv6CIDRs       string
	LokiURL                  string
	PrometheusURL            string
	TenantRouterInternalURL  string
	PullCredential           string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// ClusterEntryFromRow maps a clusterstore row onto the registry's ClusterEntry
// view. The only conversion site for rows from clusterstore — GetEntry and
// List both call this — so a field added to either struct only needs wiring
// in one place instead of drifting copies (see the pod_subnet_ipv6_cidrs
// incident this replaced).
func ClusterEntryFromRow(row *clusterstore.Cluster) ClusterEntry {
	return ClusterEntry{
		ID:                       row.ID,
		IsDefault:                false,
		Region:                   row.Region,
		EKSClusterName:           row.EKSClusterName,
		EKSClusterEndpoint:       row.EKSClusterEndpoint,
		EKSClusterCA:             row.EKSClusterCA,
		AgentIngressDomain:       row.AgentIngressDomain,
		AgentPublicIngressDomain: row.AgentPublicIngressDomain,
		IngestionIngressDomain:   row.IngestionIngressDomain,
		LangfuseBaseURLExt:       row.LangfuseBaseURLExt,
		LangfuseVPCEIPs:          row.LangfuseVPCEIPs,
		PodSubnetCIDRs:           row.PodSubnetCIDRs,
		PodSubnetIPv6CIDRs:       row.PodSubnetIPv6CIDRs,
		LokiURL:                  row.LokiURL,
		PrometheusURL:            row.PrometheusURL,
		TenantRouterInternalURL:  row.TenantRouterInternalURL,
		PullCredential:           row.PullCredential,
		CreatedAt:                row.CreatedAt,
		UpdatedAt:                row.UpdatedAt,
	}
}

// RegistryConfig is the process-level Kubernetes configuration that the
// default ClusterClient needs. PrometheusURL is the one exception: it feeds
// the shared Prometheus client used for deployments with no explicit
// cluster_id, sourced straight from the default cluster's config entry.
type RegistryConfig struct {
	Mode             ClientMode
	Region           string
	KubeconfigPath   string
	KubeContext      string
	EKSBootstrapName string
	EKSBootstrapURL  string
	EKSBootstrapCA   []byte
	DefaultClusterID string
	PrometheusURL    string
}

// NewRegistry builds the registry's default ClusterClient from cfg. Returns
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
		ClusterCA:       cfg.EKSBootstrapCA,
		KubeconfigPath:  cfg.KubeconfigPath,
		KubeContext:     cfg.KubeContext,
		Logger:          log,
	})
	if err != nil {
		return nil, fmt.Errorf("registry: build default cluster client: %w", err)
	}
	return &Registry{
		primary:          client,
		defaultClusterID: cfg.DefaultClusterID,
		clusterStore:     clusterStore,
		log:              log,
		cache:            make(map[string]ClusterClient),
		entryCache:       make(map[string]ClusterEntry),
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
		primary:          primary,
		defaultClusterID: cfg.DefaultClusterID,
		clusterStore:     clusterStore,
		log:              logger.New("error", "json"),
		cache:            make(map[string]ClusterClient),
	}
}

// Default returns the ClusterClient for the cluster astro-server is running
// on. Used by every handler and worker without a per-deployment cluster_id.
func (r *Registry) Default() ClusterClient {
	if r == nil {
		return nil
	}
	return r.primary
}

func (r *Registry) DefaultClusterID() string {
	if r == nil {
		return ""
	}
	return r.defaultClusterID
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
// default cluster. Returns ErrClusterNotFound whenever there's no matching
// row — including for the default cluster: if boot sync hasn't written its
// row yet (or failed to, e.g. a validation error), callers must see that as
// a real failure instead of silently getting stale or synthesized data.
//
// Entries are cached so the deploy path (which both validates the cluster
// and resolves its ingress config) doesn't re-query for every step. Refresh
// evicts cached entries; the cluster admin RPCs already call Refresh after
// every mutation.
func (r *Registry) GetEntry(ctx context.Context, id string) (ClusterEntry, error) {
	if r == nil {
		return ClusterEntry{}, fmt.Errorf("registry: nil")
	}
	if id == "" {
		id = r.defaultClusterID
	}
	if id == "" {
		return ClusterEntry{}, ErrClusterNotFound
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
	entry.IsDefault = row.ID == r.defaultClusterID
	r.mu.Lock()
	if r.entryCache == nil {
		r.entryCache = make(map[string]ClusterEntry)
	}
	r.entryCache[id] = entry
	r.mu.Unlock()
	return entry, nil
}

// List returns every cluster row from clusterstore verbatim — including (or
// excluding, if it has no row) the default cluster; no synthesized entry is
// injected. A default cluster missing its row simply doesn't appear, same as
// any other cluster would.
func (r *Registry) List(ctx context.Context) ([]ClusterEntry, error) {
	if r == nil {
		return nil, fmt.Errorf("registry: nil")
	}
	if r.clusterStore == nil {
		return nil, nil
	}

	rows, err := r.clusterStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("registry.List: %w", err)
	}
	out := make([]ClusterEntry, 0, len(rows))
	for _, row := range rows {
		entry := ClusterEntryFromRow(row)
		entry.IsDefault = row.ID == r.defaultClusterID
		out = append(out, entry)
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

// Refresh drops a cached cluster client so the next Get/GetEntry re-reads the row.
func (r *Registry) Refresh(_ context.Context, id string) error {
	if r == nil {
		return fmt.Errorf("registry: nil")
	}
	if id == "" {
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
