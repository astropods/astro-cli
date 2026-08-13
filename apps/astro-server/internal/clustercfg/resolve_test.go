package clustercfg

import (
	"context"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

func envDefaults() config.DeploymentConfig {
	return config.DeploymentConfig{
		IngressDomain:          "primary.agents.example.com",
		IngestionIngressDomain: "primary.ingestion.example.com",
	}
}

func TestResolve_PrimaryLangfuseURLPrefersExt(t *testing.T) {
	dep := envDefaults()
	dep.LangfuseBaseURL = "http://langfuse.internal:3000"
	dep.LangfuseBaseURLExt = "http://langfuse.platform.astroids.ai:3000"
	got, err := Resolve(context.Background(), nil, dep, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LangfuseBaseURL != dep.LangfuseBaseURLExt {
		t.Errorf("langfuse url: got %q want ext %q", got.LangfuseBaseURL, dep.LangfuseBaseURLExt)
	}
}

func TestResolve_PrimaryLangfuseURLFallsBackToBase(t *testing.T) {
	dep := envDefaults()
	dep.LangfuseBaseURL = "http://langfuse.internal:3000"
	dep.LangfuseBaseURLExt = ""
	got, err := Resolve(context.Background(), nil, dep, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LangfuseBaseURL != dep.LangfuseBaseURL {
		t.Errorf("langfuse url: got %q want base %q", got.LangfuseBaseURL, dep.LangfuseBaseURL)
	}
}

func TestResolve_PrimaryUsesEnvDefaults(t *testing.T) {
	got, err := Resolve(context.Background(), nil, envDefaults(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentIngressDomain != "primary.agents.example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_PrimaryCPSubnetCIDRs(t *testing.T) {
	dep := envDefaults()
	dep.PodSubnetCIDRs = []string{"100.65.0.0/20", "100.65.16.0/20"}
	dep.CPSubnetCIDRs = []string{"10.3.11.0/24", "10.3.12.0/24"}

	got, err := Resolve(context.Background(), nil, dep, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.PodSubnetCIDRs) != 2 || got.PodSubnetCIDRs[0] != "100.65.0.0/20" {
		t.Errorf("pod subnet cidrs: %v", got.PodSubnetCIDRs)
	}
	if len(got.CPSubnetCIDRs) != 2 || got.CPSubnetCIDRs[0] != "10.3.11.0/24" {
		t.Errorf("cp subnet cidrs: %v", got.CPSubnetCIDRs)
	}
}

// TestResolve_NoDefaultClusterConfiguredUsesEnvDefaults covers local dev:
// reg is non-nil (a real ClusterClient backs Default()) but DEFAULT_CLUSTER_ID
// is unset, so there's no cluster-config row to validate against — clusterID
// == "" must still fall back to dep instead of erroring via GetEntry.
func TestResolve_NoDefaultClusterConfiguredUsesEnvDefaults(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	got, err := Resolve(context.Background(), reg, envDefaults(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentIngressDomain != "primary.agents.example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_AdditionalClusterUsesEntryVerbatim(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	full := k8s.ClusterEntry{
		ID:                     "eu-west-1",
		AgentIngressDomain:     "eu.agents.example.com",
		IngestionIngressDomain: "eu.ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10,10.0.2.10",
		PodSubnetCIDRs:         "10.0.0.0/24,10.1.0.0/24",
	}
	reg.SetCachedEntryForTest("eu-west-1", full)

	got, err := Resolve(context.Background(), reg, envDefaults(), "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentIngressDomain != "eu.agents.example.com" {
		t.Errorf("ingress: %s", got.AgentIngressDomain)
	}
	if got.LangfuseBaseURL != "http://langfuse.platform.astroids.ai:3000" {
		t.Errorf("langfuse url: %s", got.LangfuseBaseURL)
	}
	if len(got.LangfuseVPCEIPs) != 2 || got.LangfuseVPCEIPs[0] != "10.0.1.10" || got.LangfuseVPCEIPs[1] != "10.0.2.10" {
		t.Errorf("langfuse vpce ips: %v", got.LangfuseVPCEIPs)
	}
	if len(got.PodSubnetCIDRs) != 2 || got.PodSubnetCIDRs[0] != "10.0.0.0/24" || got.PodSubnetCIDRs[1] != "10.1.0.0/24" {
		t.Errorf("pod subnet cidrs: %v", got.PodSubnetCIDRs)
	}
}

// TestResolve_DefaultClusterRowAllowsEmptyVPCEIPs covers a synced default
// cluster row (unlike TestResolve_NoDefaultClusterConfiguredUsesEnvDefaults,
// which covers no row at all): same-region default clusters don't need a
// PrivateLink netpol exception to reach Langfuse, so their row can carry an
// empty langfuse_vpce_ips.
func TestResolve_DefaultClusterRowAllowsEmptyVPCEIPs(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("us-east-1-managed", k8s.ClusterEntry{
		ID:                     "us-east-1-managed",
		IsDefault:              true,
		AgentIngressDomain:     "agents.example.com",
		IngestionIngressDomain: "ingestion.example.com",
		LangfuseBaseURLExt:     "https://langfuse.example.com",
		LangfuseVPCEIPs:        "",
		PodSubnetCIDRs:         "10.0.0.0/24",
	})

	got, err := Resolve(context.Background(), reg, envDefaults(), "us-east-1-managed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.LangfuseVPCEIPs) != 0 {
		t.Errorf("langfuse vpce ips: %v", got.LangfuseVPCEIPs)
	}
}

// TestResolve_AdditionalClusterEmptyVPCEIPsOK covers every cluster, not just
// the default one: langfuse_vpce_ips is optional everywhere, so an empty (or
// whitespace-only, once parsed) value at resolve time is never an error —
// any format problem was already caught at write time in clusterstore.
func TestResolve_AdditionalClusterEmptyVPCEIPsOK(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                     "eu-west-1",
		AgentIngressDomain:     "eu.agents.example.com",
		IngestionIngressDomain: "eu.ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        " , ",
		PodSubnetCIDRs:         "10.0.0.0/24",
	})

	got, err := Resolve(context.Background(), reg, envDefaults(), "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.LangfuseVPCEIPs) != 0 {
		t.Errorf("langfuse vpce ips: %v", got.LangfuseVPCEIPs)
	}
}

func TestResolve_AdditionalClusterWithEmptyFieldErrors(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                 "eu-west-1",
		AgentIngressDomain: "eu.agents.example.com",
		// other fields intentionally left blank
	})

	_, err := Resolve(context.Background(), reg, envDefaults(), "eu-west-1")
	if err == nil {
		t.Fatal("expected error for empty ingress field")
	}
	if !strings.Contains(err.Error(), "ingestion_ingress_domain") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
}

func TestResolve_PrimaryUsesConfiguredPullCredential(t *testing.T) {
	dep := envDefaults()
	dep.RegistryPullCredential = "astrocp_primary_secret"

	got, err := Resolve(context.Background(), nil, dep, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RegistryPullCredential != "astrocp_primary_secret" {
		t.Errorf("RegistryPullCredential = %q, want dep's configured credential", got.RegistryPullCredential)
	}
}

func TestResolve_AdditionalClusterUsesOwnPullCredential(t *testing.T) {
	dep := envDefaults()
	dep.ProxyRegistryHost = "registry.example.com"
	dep.RegistryPullCredential = "astrocp_primary_secret"

	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                     "eu-west-1",
		AgentIngressDomain:     "eu.agents.example.com",
		IngestionIngressDomain: "eu.ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10",
		PodSubnetCIDRs:         "10.0.0.0/24",
		PullCredential:         "astrocp_eu-west-1_secret",
	})

	got, err := Resolve(context.Background(), reg, dep, "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RegistryPullCredential != "astrocp_eu-west-1_secret" {
		t.Errorf("RegistryPullCredential = %q, want the cluster's own credential, not the primary's", got.RegistryPullCredential)
	}
}

func TestResolve_AdditionalClusterMissingPullCredentialErrorsWhenPullThroughEnabled(t *testing.T) {
	dep := envDefaults()
	dep.ProxyRegistryHost = "registry.example.com"

	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                     "eu-west-1",
		AgentIngressDomain:     "eu.agents.example.com",
		IngestionIngressDomain: "eu.ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10",
		PodSubnetCIDRs:         "10.0.0.0/24",
	})

	_, err := Resolve(context.Background(), reg, dep, "eu-west-1")
	if err == nil || !strings.Contains(err.Error(), "pull credential") {
		t.Fatalf("expected a pull credential error, got %v", err)
	}
}

func TestResolve_AdditionalClusterMissingPullCredentialOKWhenPullThroughDisabled(t *testing.T) {
	dep := envDefaults() // ProxyRegistryHost unset, matching local dev

	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                     "eu-west-1",
		AgentIngressDomain:     "eu.agents.example.com",
		IngestionIngressDomain: "eu.ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10",
		PodSubnetCIDRs:         "10.0.0.0/24",
	})

	got, err := Resolve(context.Background(), reg, dep, "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error when pull-through is disabled: %v", err)
	}
	if got.RegistryPullCredential != "" {
		t.Errorf("RegistryPullCredential = %q, want empty", got.RegistryPullCredential)
	}
}

func TestResolve_UnknownClusterErrors(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	_, err := Resolve(context.Background(), reg, envDefaults(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown cluster")
	}
}
