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
