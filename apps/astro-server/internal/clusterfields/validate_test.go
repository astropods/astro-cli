package clusterfields

import (
	"strings"
	"testing"
)

func fullDeploy() DeployConfig {
	return DeployConfig{
		AgentIngressDomain:     "agents.example.com",
		IngestionIngressDomain: "ingest.example.com",
		KnowledgeDomain:        "knowledge.example.com",
		LangfuseBaseURLExt:     "http://langfuse.example:3000",
		LangfuseVPCEIPs:        "10.0.0.1",
		PodSubnetCIDRs:         "10.0.0.0/24",
	}
}

func TestValidateDeployNonEmpty_ok(t *testing.T) {
	if err := ValidateDeployNonEmpty("eu", fullDeploy()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDeployNonEmpty_storeError(t *testing.T) {
	d := fullDeploy()
	d.AgentIngressDomain = ""
	err := ValidateDeployNonEmpty("", d)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent_ingress_domain is required") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDeployNonEmpty_deployError(t *testing.T) {
	d := fullDeploy()
	d.AgentIngressDomain = ""
	err := ValidateDeployNonEmpty("eu-west-1", d)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cluster "eu-west-1" is missing required field agent_ingress_domain`) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDeployNonEmpty_whitespaceVPCEIPs(t *testing.T) {
	d := fullDeploy()
	d.LangfuseVPCEIPs = " , "
	err := ValidateDeployNonEmpty("eu", d)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "langfuse_vpce_ips") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationNonEmpty_requiresEKS(t *testing.T) {
	r := Registration{Deploy: fullDeploy()}
	err := ValidateRegistrationNonEmpty(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "region is required") {
		t.Fatalf("got %v", err)
	}
}
