package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"golang.org/x/sync/singleflight"
)

const (
	runtimeStorageTTL          = 15 * time.Second
	runtimeStorageFetchTimeout = 3 * time.Second
)

var runtimeStorageCache = newStorageStatsCache(runtimeStorageTTL)

// workloadStorage is a workload's representative-pod data-volume usage.
type workloadStorage struct {
	usedBytes     int64
	capacityBytes int64
}

// storageStatsCache memoizes per-deployment storage; singleflight collapses
// concurrent misses into one Prometheus query.
type storageStatsCache struct {
	ttl  time.Duration
	sf   singleflight.Group
	mu   sync.RWMutex
	data map[string]storageCacheEntry
}

type storageCacheEntry struct {
	stats     map[string]workloadStorage
	expiresAt time.Time
}

func newStorageStatsCache(ttl time.Duration) *storageStatsCache {
	return &storageStatsCache{ttl: ttl, data: make(map[string]storageCacheEntry)}
}

// get returns cached stats or invokes load once across concurrent callers. A
// failed load still caches its empty result for the TTL, so a struggling
// Prometheus isn't hammered every poll.
func (c *storageStatsCache) get(key string, now time.Time, load func() map[string]workloadStorage) map[string]workloadStorage {
	c.mu.RLock()
	entry, ok := c.data[key]
	c.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.stats
	}

	v, _, _ := c.sf.Do(key, func() (any, error) {
		c.mu.RLock()
		entry, ok := c.data[key]
		c.mu.RUnlock()
		if ok && now.Before(entry.expiresAt) {
			return entry.stats, nil
		}
		stats := load()
		c.mu.Lock()
		c.data[key] = storageCacheEntry{stats: stats, expiresAt: now.Add(c.ttl)}
		// Drop entries untouched for over a TTL so the map stays bounded.
		for k, e := range c.data {
			if e.expiresAt.Before(now.Add(-c.ttl)) {
				delete(c.data, k)
			}
		}
		c.mu.Unlock()
		return stats, nil
	})
	stats, _ := v.(map[string]workloadStorage)
	return stats
}

// statefulSetWorkloadNames scopes the storage query: StatefulSets are the only
// workloads that own a persistent volume.
func statefulSetWorkloadNames(snap *deploymentstore.RuntimeSnapshot) map[string]bool {
	if snap == nil {
		return nil
	}
	out := make(map[string]bool)
	for _, w := range snap.Workloads {
		if w.Kind == "StatefulSet" {
			out[w.Name] = true
		}
	}
	return out
}

// enrichRuntimeStorage overlays per-workload volume usage from a cached,
// per-deployment Prometheus query. Storage is left absent when Prometheus is
// unavailable or nothing persistent is running.
func enrichRuntimeStorage(
	ctx context.Context,
	cache *storageStatsCache,
	promClient *promquery.Client,
	clusterFilter string,
	deploymentID string,
	namespace string,
	statefulSetWorkloads map[string]bool,
	rt *DeploymentRuntime,
) {
	if promClient == nil || rt == nil || len(rt.Workloads) == 0 {
		return
	}

	// A StatefulSet's data-volume PVC is deterministically named "data-<pod>"
	// (agentDataVolumeName); map each back to its workload and skip the rest.
	pvcToWorkload := make(map[string]string, len(rt.Workloads))
	for _, w := range rt.Workloads {
		if w.PodName == "" || !statefulSetWorkloads[w.Name] {
			continue
		}
		pvcToWorkload["data-"+w.PodName] = w.Name
	}
	if len(pvcToWorkload) == 0 {
		return
	}

	stats := cache.get(deploymentID, time.Now(), func() map[string]workloadStorage {
		// Detach from the caller's cancellation: the flight is shared, so one
		// client leaving mustn't cancel it for every viewer.
		qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeStorageFetchTimeout)
		defer cancel()
		return queryWorkloadStorage(qctx, promClient, namespace, clusterFilter, pvcToWorkload)
	})

	for i := range rt.Workloads {
		s, ok := stats[rt.Workloads[i].Name]
		if !ok {
			continue
		}
		used, capacity := s.usedBytes, s.capacityBytes
		rt.Workloads[i].StorageUsedBytes = &used
		rt.Workloads[i].StorageCapacityBytes = &capacity
	}
}

// queryWorkloadStorage runs the used/capacity queries and folds per-PVC results
// onto workloads. Returns an empty map on any error so the caller backs off.
func queryWorkloadStorage(
	ctx context.Context,
	promClient *promquery.Client,
	namespace string,
	clusterFilter string,
	pvcToWorkload map[string]string,
) map[string]workloadStorage {
	out := make(map[string]workloadStorage)

	pvcNames := make([]string, 0, len(pvcToWorkload))
	for pvc := range pvcToWorkload {
		pvcNames = append(pvcNames, pvc)
	}
	usedQL, capQL := buildStorageStatsQueries(namespace, pvcNames, clusterFilter)
	if usedQL == "" {
		return out
	}

	usedSamples, err := promClient.Query(ctx, usedQL)
	if err != nil {
		return out
	}
	capSamples, err := promClient.Query(ctx, capQL)
	if err != nil {
		return out
	}

	fold := func(samples []promquery.Sample, add func(*workloadStorage, int64)) {
		for _, s := range samples {
			wl, ok := pvcToWorkload[s.Labels["persistentvolumeclaim"]]
			if !ok {
				continue
			}
			ws := out[wl]
			add(&ws, int64(s.Value))
			out[wl] = ws
		}
	}
	fold(usedSamples, func(ws *workloadStorage, v int64) { ws.usedBytes += v })
	fold(capSamples, func(ws *workloadStorage, v int64) { ws.capacityBytes += v })
	return out
}

// buildStorageStatsQueries builds the used/capacity queries without sum(), so
// each series keeps its persistentvolumeclaim label for folding.
func buildStorageStatsQueries(namespace string, pvcNames []string, clusterFilter string) (string, string) {
	if len(pvcNames) == 0 {
		return "", ""
	}
	parts := make([]string, 0, len(pvcNames))
	for _, n := range pvcNames {
		parts = append(parts, regexEscape(n))
	}
	selector := fmt.Sprintf(`namespace="%s",persistentvolumeclaim=~"%s"%s`, namespace, strings.Join(parts, "|"), clusterFilter)
	used := fmt.Sprintf(`kubelet_volume_stats_used_bytes{%s}`, selector)
	capacity := fmt.Sprintf(`kubelet_volume_stats_capacity_bytes{%s}`, selector)
	return used, capacity
}
