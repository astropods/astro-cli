package k8s

import (
	"context"
	"testing"
)

func TestPrometheusClusterFilter_PrimaryAndAdditional(t *testing.T) {
	reg := &Registry{
		regCfg: RegistryConfig{EKSBootstrapName: "primary-eks"},
	}
	reg.SetCachedEntryForTest("eu", ClusterEntry{
		ID:             "eu",
		EKSClusterName: "preview-eu-eks",
		Enabled:        true,
	})

	if got := reg.PrometheusClusterFilter(context.Background(), ""); got != `,cluster="primary-eks"` {
		t.Fatalf("primary filter = %q", got)
	}
	if got := reg.PrometheusClusterFilter(context.Background(), "eu"); got != `,cluster="preview-eu-eks"` {
		t.Fatalf("eu filter = %q", got)
	}
}

func TestLokiClusterName(t *testing.T) {
	reg := &Registry{regCfg: RegistryConfig{EKSBootstrapName: "primary-eks"}}
	reg.SetCachedEntryForTest("eu", ClusterEntry{ID: "eu", EKSClusterName: "preview-eu-eks", Enabled: true})

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
	reg := &Registry{regCfg: RegistryConfig{EKSBootstrapName: "primary-eks"}}
	reg.SetCachedEntryForTest("blank", ClusterEntry{ID: "blank", EKSClusterName: "", Enabled: true})
	ctx := context.Background()

	if got := reg.PrometheusClusterFilter(ctx, "blank"); got != "" {
		t.Fatalf("blank EKSClusterName PrometheusClusterFilter = %q, want empty", got)
	}
	if got := reg.LokiClusterName(ctx, "blank"); got != "" {
		t.Fatalf("blank EKSClusterName LokiClusterName = %q, want empty", got)
	}
}
