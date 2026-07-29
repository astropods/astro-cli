package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

func TestBuildStorageStatsQueries(t *testing.T) {
	if used, capacity := buildStorageStatsQueries("ns", nil, ""); used != "" || capacity != "" {
		t.Fatalf("expected empty queries for no PVCs, got %q / %q", used, capacity)
	}

	used, capacity := buildStorageStatsQueries("ns-1", []string{"data-kn-0", "data-agent-0"}, `,cluster="c1"`)
	// No sum(): each series must keep its PVC label for folding.
	if strings.Contains(used, "sum(") || strings.Contains(capacity, "sum(") {
		t.Fatalf("queries must not wrap in sum(): %q / %q", used, capacity)
	}
	for _, want := range []string{`namespace="ns-1"`, `persistentvolumeclaim=~"data-kn-0|data-agent-0"`, `cluster="c1"`} {
		if !strings.Contains(used, want) {
			t.Errorf("used query %q missing %q", used, want)
		}
	}
	if !strings.HasPrefix(used, "kubelet_volume_stats_used_bytes{") {
		t.Errorf("unexpected used metric: %q", used)
	}
	if !strings.HasPrefix(capacity, "kubelet_volume_stats_capacity_bytes{") {
		t.Errorf("unexpected capacity metric: %q", capacity)
	}
}

func TestStorageStatsCacheTTL(t *testing.T) {
	c := newStorageStatsCache(10 * time.Second)
	base := time.Unix(1_700_000_000, 0)
	var calls int
	load := func() map[string]workloadStorage {
		calls++
		return map[string]workloadStorage{"a": {usedBytes: int64(calls)}}
	}

	c.get("dep", base, load)
	c.get("dep", base.Add(5*time.Second), load)
	if calls != 1 {
		t.Fatalf("expected 1 load within TTL, got %d", calls)
	}
	got := c.get("dep", base.Add(11*time.Second), load)
	if calls != 2 {
		t.Fatalf("expected reload after TTL, got %d loads", calls)
	}
	if got["a"].usedBytes != 2 {
		t.Fatalf("expected refreshed value, got %d", got["a"].usedBytes)
	}
}

func TestStorageStatsCacheSingleflight(t *testing.T) {
	c := newStorageStatsCache(time.Minute)
	base := time.Unix(1_700_000_000, 0)
	var calls int32
	load := func() map[string]workloadStorage {
		atomic.AddInt32(&calls, 1)
		time.Sleep(30 * time.Millisecond) // hold the flight open so misses pile up
		return map[string]workloadStorage{}
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			<-start
			c.get("dep", base, load)
		})
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected concurrent misses to collapse to 1 load, got %d", got)
	}
}

// fakeProm serves one labeled series per metric for the given PVC.
func fakeProm(t *testing.T, pvc string, usedBytes, capacityBytes int64) *promquery.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		val := "0"
		switch {
		case strings.Contains(q, "used_bytes"):
			val = fmt.Sprintf("%d", usedBytes)
		case strings.Contains(q, "capacity_bytes"):
			val = fmt.Sprintf("%d", capacityBytes)
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"persistentvolumeclaim":%q},"value":[1700000000,%q]}]}}`, pvc, val)
	}))
	t.Cleanup(srv.Close)
	return promquery.NewClient(srv.URL, "")
}

func TestEnrichRuntimeStorage(t *testing.T) {
	const usedGiB3 = int64(3) << 30
	const capGiB10 = int64(10) << 30
	client := fakeProm(t, "data-knowledge-qdrant-0", usedGiB3, capGiB10)

	rt := &DeploymentRuntime{Workloads: []WorkloadRuntime{
		{Name: "knowledge-qdrant", PodName: "knowledge-qdrant-0"}, // PVC matches → enriched
		{Name: "agent", PodName: "agent-0"},                       // no matching series → stays nil
		{Name: "ingestion", PodName: ""},                          // no pod → skipped
	}}

	enrichRuntimeStorage(context.Background(), newStorageStatsCache(time.Minute), client, "", "dep-1", "ns-1", map[string]bool{"knowledge-qdrant": true, "agent": true}, rt)

	kn := rt.Workloads[0]
	if kn.StorageUsedBytes == nil || kn.StorageCapacityBytes == nil {
		t.Fatalf("expected knowledge workload enriched, got used=%v cap=%v", kn.StorageUsedBytes, kn.StorageCapacityBytes)
	}
	if *kn.StorageUsedBytes != usedGiB3 || *kn.StorageCapacityBytes != capGiB10 {
		t.Fatalf("wrong bytes: used=%d cap=%d", *kn.StorageUsedBytes, *kn.StorageCapacityBytes)
	}
	if rt.Workloads[1].StorageUsedBytes != nil || rt.Workloads[2].StorageUsedBytes != nil {
		t.Fatalf("expected non-matching workloads to stay unset")
	}
}

func TestEnrichRuntimeStorageNoPromClient(t *testing.T) {
	rt := &DeploymentRuntime{Workloads: []WorkloadRuntime{{Name: "knowledge-qdrant", PodName: "knowledge-qdrant-0"}}}
	enrichRuntimeStorage(context.Background(), newStorageStatsCache(time.Minute), nil, "", "dep-1", "ns-1", map[string]bool{"knowledge-qdrant": true}, rt)
	if rt.Workloads[0].StorageUsedBytes != nil {
		t.Fatalf("expected no enrichment when prom client is nil")
	}
}

// A storage-less deployment (no StatefulSet) must fire no Prometheus query.
func TestEnrichRuntimeStorageSkipsNonStatefulSets(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	t.Cleanup(srv.Close)
	client := promquery.NewClient(srv.URL, "")

	rt := &DeploymentRuntime{Workloads: []WorkloadRuntime{
		{Name: "agent", PodName: "agent-7d9f8b-2x9k"},
		{Name: "collector", PodName: "collector-64bc9-lm4p"},
	}}
	enrichRuntimeStorage(context.Background(), newStorageStatsCache(time.Minute), client, "", "dep-x", "ns-x", map[string]bool{}, rt)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected no Prometheus query for a storage-less deployment, got %d", got)
	}
	if rt.Workloads[0].StorageUsedBytes != nil {
		t.Fatalf("expected no storage on non-StatefulSet workloads")
	}
}
