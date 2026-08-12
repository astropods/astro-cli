package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
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

// LokiClientFor returns the Loki client for a deployment's cluster: the
// cluster's own loki_url override (see ClusterEntry.LokiURL) if it has one,
// otherwise defaultClient — the global LOKI_URL client every cluster used
// before per-cluster overrides existed. Nil-safe: a nil registry or a
// resolution error both fall back to defaultClient.
//
// Resolves the entry via GetEntry, which is a single lookup for a caller that
// only has a cluster id in hand (the common case: a deployment's
// EffectiveClusterID). A caller that already has the ClusterEntry — e.g. one
// iterating Registry.List — should call LokiClientForEntry directly instead;
// GetEntry doesn't share List's cache, so calling this per entry in a loop
// costs a redundant clusterstore round trip per cluster.
func (r *Registry) LokiClientFor(ctx context.Context, deploymentClusterID string, defaultClient *loki.Client) *loki.Client {
	if r == nil {
		return defaultClient
	}
	entry, err := r.GetEntry(ctx, deploymentClusterID)
	if err != nil {
		return defaultClient
	}
	return r.LokiClientForEntry(entry, defaultClient)
}

// LokiClientForEntry is LokiClientFor for a caller that already has the
// ClusterEntry (e.g. from Registry.List), avoiding a redundant GetEntry call.
func (r *Registry) LokiClientForEntry(entry ClusterEntry, defaultClient *loki.Client) *loki.Client {
	if r == nil || entry.LokiURL == "" {
		return defaultClient
	}
	return r.cachedLokiClient(entry.ID, entry.LokiURL)
}

// PrometheusClientFor returns the Prometheus/VictoriaMetrics client for a
// deployment's cluster: the cluster's own prometheus_url override if it has
// one (labeled with its EKS cluster name, same as PrometheusClusterFilter),
// otherwise defaultClient. Nil-safe like LokiClientFor; see its doc comment
// for when to prefer PrometheusClientForEntry instead.
func (r *Registry) PrometheusClientFor(ctx context.Context, deploymentClusterID string, defaultClient *promquery.Client) *promquery.Client {
	if r == nil {
		return defaultClient
	}
	entry, err := r.GetEntry(ctx, deploymentClusterID)
	if err != nil {
		return defaultClient
	}
	return r.PrometheusClientForEntry(entry, defaultClient)
}

// PrometheusClientForEntry is PrometheusClientFor for a caller that already
// has the ClusterEntry (e.g. from Registry.List), avoiding a redundant
// GetEntry call.
func (r *Registry) PrometheusClientForEntry(entry ClusterEntry, defaultClient *promquery.Client) *promquery.Client {
	if r == nil || entry.PrometheusURL == "" {
		return defaultClient
	}
	return r.cachedPromClient(entry.ID, entry.PrometheusURL, entry.EKSClusterName)
}

func (r *Registry) cachedLokiClient(id, url string) *loki.Client {
	r.mu.RLock()
	c, ok := r.lokiCache[id]
	r.mu.RUnlock()
	if ok {
		return c
	}

	c = loki.New(url)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.lokiCache[id]; ok {
		return existing
	}
	if r.lokiCache == nil {
		r.lokiCache = make(map[string]*loki.Client)
	}
	r.lokiCache[id] = c
	return c
}

func (r *Registry) cachedPromClient(id, url, cluster string) *promquery.Client {
	r.mu.RLock()
	c, ok := r.promCache[id]
	r.mu.RUnlock()
	if ok {
		return c
	}

	c = promquery.NewClient(url, cluster)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.promCache[id]; ok {
		return existing
	}
	if r.promCache == nil {
		r.promCache = make(map[string]*promquery.Client)
	}
	r.promCache[id] = c
	return c
}
