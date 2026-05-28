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
		ACMCertificateARN:      "arn:primary",
		ALBGroupName:           "primary",
		IngestionIngressDomain: "primary.ingestion.example.com",
		IngestionACMCertARN:    "arn:primary-ingest",
		IngestionALBGroupName:  "primary-ingest",
		KnowledgeDomain:        "primary.knowledge.example.com",
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

func TestResolve_PrimaryIDUsesEnvDefaults(t *testing.T) {
	got, err := Resolve(context.Background(), nil, envDefaults(), k8s.PrimaryClusterID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.KnowledgeDomain != "primary.knowledge.example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_AdditionalClusterUsesEntryVerbatim(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	full := k8s.ClusterEntry{
		ID:                     "eu-west-1",
		Enabled:                true,
		AgentIngressDomain:     "eu.agents.example.com",
		AgentACMCertARN:        "arn:eu",
		AgentALBGroupName:      "eu-agents",
		IngestionIngressDomain: "eu.ingestion.example.com",
		IngestionACMCertARN:    "arn:eu-ingest",
		IngestionALBGroupName:  "eu-ingest",
		KnowledgeDomain:        "eu.knowledge.example.com",
	}
	reg.SetCachedEntryForTest("eu-west-1", full)

	got, err := Resolve(context.Background(), reg, envDefaults(), "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentIngressDomain != "eu.agents.example.com" {
		t.Errorf("ingress: %s", got.AgentIngressDomain)
	}
	if got.KnowledgeDomain != "eu.knowledge.example.com" {
		t.Errorf("knowledge: %s", got.KnowledgeDomain)
	}
	// Env defaults must not leak through for a non-primary cluster.
	if got.AgentACMCertARN == "arn:primary" {
		t.Errorf("env default leaked into ACM ARN")
	}
}

func TestResolve_AdditionalClusterWithEmptyFieldErrors(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	reg.SetCachedEntryForTest("eu-west-1", k8s.ClusterEntry{
		ID:                 "eu-west-1",
		Enabled:            true,
		AgentIngressDomain: "eu.agents.example.com",
		AgentACMCertARN:    "arn:eu",
		// other fields intentionally left blank
	})

	_, err := Resolve(context.Background(), reg, envDefaults(), "eu-west-1")
	if err == nil {
		t.Fatal("expected error for empty ingress field")
	}
	if !strings.Contains(err.Error(), "agent_alb_group_name") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
}

func TestResolve_UnknownClusterErrors(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)
	_, err := Resolve(context.Background(), reg, envDefaults(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown cluster")
	}
}
