package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClusterClient abstracts Kubernetes cluster connectivity.
// Implementations: EKSClient (production) and LocalClient (development).
type ClusterClient interface {
	Clientset() *kubernetes.Clientset
	Config() *rest.Config
	CheckHealth() error
	GetServerVersion() (string, error)
	DiagnoseConnection() map[string]string
}

// ClientMode selects the Kubernetes client implementation.
type ClientMode string

const (
	ClientModeEKS   ClientMode = "eks"
	ClientModeLocal ClientMode = "local"
)

// ClusterClientConfig holds configuration for creating a ClusterClient.
type ClusterClientConfig struct {
	Mode ClientMode

	// EKS-specific
	ClusterName     string
	ClusterEndpoint string
	Region          string

	// Local-specific
	KubeconfigPath string
	KubeContext    string

	Logger *logger.Logger
}

// NewClusterClient creates a ClusterClient based on the configured mode.
func NewClusterClient(ctx context.Context, cfg ClusterClientConfig) (ClusterClient, error) {
	switch cfg.Mode {
	case ClientModeLocal:
		return NewLocalClient(cfg)
	case ClientModeEKS, "":
		return NewEKSClient(ctx, EKSClientConfig{
			ClusterName:     cfg.ClusterName,
			ClusterEndpoint: cfg.ClusterEndpoint,
			Region:          cfg.Region,
			Logger:          cfg.Logger,
		})
	default:
		return nil, fmt.Errorf("unknown K8S_CLIENT_MODE: %q (must be \"eks\" or \"local\")", cfg.Mode)
	}
}
