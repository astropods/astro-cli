package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientConfig holds configuration for Kubernetes client
type ClientConfig struct {
	// Path to kubeconfig file (empty for in-cluster config)
	KubeconfigPath string
	// Whether to use in-cluster configuration
	InCluster bool
	// Master URL override
	MasterURL string
}

// Client wraps the Kubernetes clientset
type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
}

// NewClient creates a new Kubernetes client
func NewClient(cfg ClientConfig) (*Client, error) {
	var config *rest.Config
	var err error

	if cfg.InCluster {
		// Use in-cluster configuration
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else {
		// Use kubeconfig file
		kubeconfigPath := cfg.KubeconfigPath
		if kubeconfigPath == "" {
			// Default to ~/.kube/config
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}

		config, err = clientcmd.BuildConfigFromFlags(cfg.MasterURL, kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
	}, nil
}

// Clientset returns the underlying Kubernetes clientset
func (c *Client) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// Config returns the REST config
func (c *Client) Config() *rest.Config {
	return c.config
}
