package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

func TestDeploymentClusterClient_PrimaryWhenEmptyClusterID(t *testing.T) {
	primary := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	reg := k8s.NewRegistryWithPrimary(primary)
	dep := &deploymentstore.Deployment{ID: "dep-1"}

	got, err := deploymentClusterClient(context.Background(), reg, dep)
	if err != nil {
		t.Fatalf("deploymentClusterClient: %v", err)
	}
	if got != primary {
		t.Fatalf("got %p, want primary %p", got, primary)
	}
}

func TestDeploymentClusterClient_AdditionalDelegatesToGet(t *testing.T) {
	primary := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	reg := k8s.NewRegistryWithPrimary(primary)
	cid := "eu-west-1"
	dep := &deploymentstore.Deployment{ID: "dep-1", ClusterID: &cid}

	_, err := deploymentClusterClient(context.Background(), reg, dep)
	if !errors.Is(err, k8s.ErrClusterNotFound) {
		t.Fatalf("want ErrClusterNotFound, got %v", err)
	}
}

func TestClusterHealthForDeploy_Healthy(t *testing.T) {
	primary := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	reg := k8s.NewRegistryWithPrimary(primary)
	reg.SetCachedClientForTest("eu-west-1", primary)

	if err := clusterHealthForDeploy(context.Background(), reg, "eu-west-1"); err != nil {
		t.Fatalf("expected healthy cluster, got error: %v", err)
	}
}

func TestClusterHealthForDeploy_Unhealthy(t *testing.T) {
	primary := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	reg := k8s.NewRegistryWithPrimary(primary)
	reg.SetCachedClientForTest("bad", unhealthyStubCluster("connection refused"))

	err := clusterHealthForDeploy(context.Background(), reg, "bad")
	if err == nil {
		t.Fatal("expected unhealthy cluster error")
	}
	if got := k8s.PublicClusterHealthDetail(err); got != "unable to connect to cluster" {
		t.Fatalf("expected sanitized connect detail, got %q", got)
	}
}
