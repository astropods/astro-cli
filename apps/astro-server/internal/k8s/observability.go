package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

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

func (r *Registry) LokiClientForEntry(entry ClusterEntry, defaultClient *loki.Client) *loki.Client {
	if r == nil || entry.LokiURL == "" {
		return defaultClient
	}
	return r.cachedLokiClient(entry.ID, entry.LokiURL)
}

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
