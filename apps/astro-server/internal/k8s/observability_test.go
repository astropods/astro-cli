package k8s

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

func TestPrometheusClusterFilter_PrimaryAndAdditional(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("primary", ClusterEntry{ID: "primary", EKSClusterName: "primary-eks"})
	reg.SetCachedEntryForTest("eu", ClusterEntry{
		ID:             "eu",
		EKSClusterName: "preview-eu-eks",
		
	})

	if got := reg.PrometheusClusterFilter(context.Background(), ""); got != `,cluster="primary-eks"` {
		t.Fatalf("primary filter = %q", got)
	}
	if got := reg.PrometheusClusterFilter(context.Background(), "eu"); got != `,cluster="preview-eu-eks"` {
		t.Fatalf("eu filter = %q", got)
	}
}

// TestPrometheusClusterFilter_DefaultClusterNoRow covers the case that used
// to silently return a synthesized "primary-eks" filter: with no clusters
// row for the default cluster, GetEntry fails and the filter comes back
// empty instead of fabricating a cluster label from RegistryConfig.
func TestPrometheusClusterFilter_DefaultClusterNoRow(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}

	if got := reg.PrometheusClusterFilter(context.Background(), ""); got != "" {
		t.Fatalf("primary filter = %q, want empty when the default cluster has no row", got)
	}
}

func TestLokiClusterName(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("primary", ClusterEntry{ID: "primary", EKSClusterName: "primary-eks"})
	reg.SetCachedEntryForTest("eu", ClusterEntry{ID: "eu", EKSClusterName: "preview-eu-eks"})

	if got := reg.LokiClusterName(context.Background(), ""); got != "primary-eks" {
		t.Fatalf("primary loki cluster = %q", got)
	}
	if got := reg.LokiClusterName(context.Background(), "eu"); got != "preview-eu-eks" {
		t.Fatalf("eu loki cluster = %q", got)
	}
}

func TestObservabilityHelpers_NilRegistry(t *testing.T) {
	var reg *Registry
	ctx := context.Background()

	if got := reg.PrometheusClusterFilter(ctx, ""); got != "" {
		t.Fatalf("nil registry PrometheusClusterFilter = %q, want empty", got)
	}
	if got := reg.LokiClusterName(ctx, ""); got != "" {
		t.Fatalf("nil registry LokiClusterName = %q, want empty", got)
	}
}

func TestObservabilityHelpers_GetEntryError(t *testing.T) {
	reg := NewRegistryWithPrimary(nil)
	ctx := context.Background()

	if got := reg.PrometheusClusterFilter(ctx, "unknown"); got != "" {
		t.Fatalf("unknown cluster PrometheusClusterFilter = %q, want empty", got)
	}
	if got := reg.LokiClusterName(ctx, "unknown"); got != "" {
		t.Fatalf("unknown cluster LokiClusterName = %q, want empty", got)
	}
}

func TestObservabilityHelpers_EmptyEKSClusterName(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("blank", ClusterEntry{ID: "blank", EKSClusterName: ""})
	ctx := context.Background()

	if got := reg.PrometheusClusterFilter(ctx, "blank"); got != "" {
		t.Fatalf("blank EKSClusterName PrometheusClusterFilter = %q, want empty", got)
	}
	if got := reg.LokiClusterName(ctx, "blank"); got != "" {
		t.Fatalf("blank EKSClusterName LokiClusterName = %q, want empty", got)
	}
}

func TestLokiClientFor_FallsBackToDefaultWithoutOverride(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("no-override", ClusterEntry{ID: "no-override", EKSClusterName: "eks-1"})
	defaultClient := loki.New("http://default-loki:3100")

	if got := reg.LokiClientFor(context.Background(), "no-override", defaultClient); got != defaultClient {
		t.Fatalf("LokiClientFor without override = %v, want the default client", got)
	}
	if got := reg.LokiClientFor(context.Background(), "", defaultClient); got != defaultClient {
		t.Fatalf("LokiClientFor(primary) = %v, want the default client", got)
	}
	var nilReg *Registry
	if got := nilReg.LokiClientFor(context.Background(), "no-override", defaultClient); got != defaultClient {
		t.Fatalf("nil registry LokiClientFor = %v, want the default client", got)
	}
}

func TestLokiClientFor_UsesPerClusterOverride(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("eu", ClusterEntry{ID: "eu", EKSClusterName: "eks-eu", LokiURL: "http://eu-loki:3100"})
	defaultClient := loki.New("http://default-loki:3100")

	got := reg.LokiClientFor(context.Background(), "eu", defaultClient)
	if got == defaultClient || got == nil {
		t.Fatalf("LokiClientFor with override should return a distinct client, got %v", got)
	}
	// Resolving again must return the cached instance, not a fresh client.
	if again := reg.LokiClientFor(context.Background(), "eu", defaultClient); again != got {
		t.Fatalf("LokiClientFor should cache the per-cluster client")
	}
}

func TestPrometheusClientFor_FallsBackToDefaultWithoutOverride(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("no-override", ClusterEntry{ID: "no-override", EKSClusterName: "eks-1"})
	defaultClient := promquery.NewClient("http://default-prom:9090", "primary-eks")

	if got := reg.PrometheusClientFor(context.Background(), "no-override", defaultClient); got != defaultClient {
		t.Fatalf("PrometheusClientFor without override = %v, want the default client", got)
	}
	var nilReg *Registry
	if got := nilReg.PrometheusClientFor(context.Background(), "no-override", defaultClient); got != defaultClient {
		t.Fatalf("nil registry PrometheusClientFor = %v, want the default client", got)
	}
}

func TestPrometheusClientFor_UsesPerClusterOverride(t *testing.T) {
	reg := &Registry{defaultClusterID: "primary"}
	reg.SetCachedEntryForTest("eu", ClusterEntry{ID: "eu", EKSClusterName: "eks-eu", PrometheusURL: "http://eu-prom:9090"})
	defaultClient := promquery.NewClient("http://default-prom:9090", "primary-eks")

	got := reg.PrometheusClientFor(context.Background(), "eu", defaultClient)
	if got == defaultClient || got == nil {
		t.Fatalf("PrometheusClientFor with override should return a distinct client, got %v", got)
	}
	if got.Cluster() != "eks-eu" {
		t.Fatalf("PrometheusClientFor override cluster label = %q, want eks-eu", got.Cluster())
	}
	if again := reg.PrometheusClientFor(context.Background(), "eu", defaultClient); again != got {
		t.Fatalf("PrometheusClientFor should cache the per-cluster client")
	}
}
