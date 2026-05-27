package admingrpc

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type taggedClusterClient struct {
	tag string
}

func (c *taggedClusterClient) Clientset() *kubernetes.Clientset      { return nil }
func (c *taggedClusterClient) Config() *rest.Config                  { return nil }
func (c *taggedClusterClient) CheckHealth() error                    { return nil }
func (c *taggedClusterClient) GetServerVersion() (string, error)     { return c.tag, nil }
func (c *taggedClusterClient) DiagnoseConnection() map[string]string { return map[string]string{"tag": c.tag} }

func TestDeploymentClusterClient_PrimaryWhenClusterIDEmpty(t *testing.T) {
	primary := &taggedClusterClient{tag: "primary"}
	reg := k8s.NewRegistryWithPrimary(primary)
	s := &Server{k8sRegistry: reg}

	kc, err := s.deploymentClusterClient(context.Background(), &deploymentstore.Deployment{})
	if err != nil {
		t.Fatalf("deploymentClusterClient: %v", err)
	}
	if got, _ := kc.GetServerVersion(); got != "primary" {
		t.Fatalf("cluster client = %q, want primary", got)
	}
}

func TestDeploymentClusterClient_AdditionalCluster(t *testing.T) {
	primary := &taggedClusterClient{tag: "primary"}
	eu := &taggedClusterClient{tag: "eu-west-1-managed"}
	reg := k8s.NewRegistryWithPrimary(primary)
	reg.SetCachedClientForTest("eu-west-1-managed", eu)

	clusterID := "eu-west-1-managed"
	s := &Server{k8sRegistry: reg}

	kc, err := s.deploymentClusterClient(context.Background(), &deploymentstore.Deployment{
		ClusterID: &clusterID,
	})
	if err != nil {
		t.Fatalf("deploymentClusterClient: %v", err)
	}
	if got, _ := kc.GetServerVersion(); got != "eu-west-1-managed" {
		t.Fatalf("cluster client = %q, want eu-west-1-managed", got)
	}
}

func TestDeploymentClusterClient_LegacySingleClientFallback(t *testing.T) {
	legacy := &taggedClusterClient{tag: "legacy"}
	s := &Server{k8sClient: legacy}

	kc, err := s.deploymentClusterClient(context.Background(), nil)
	if err != nil {
		t.Fatalf("deploymentClusterClient: %v", err)
	}
	if got, _ := kc.GetServerVersion(); got != "legacy" {
		t.Fatalf("cluster client = %q, want legacy", got)
	}
}
