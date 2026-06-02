package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// LocalMessagingNodePort is the fixed K8s NodePort published on the messaging
// Service in local mode. Local mode has no ingress, so the Launch button
// instead links to http://localhost:<LocalMessagingNodePort>/, which
// docker-desktop / k3d map straight to the host. The port lives in the
// default NodePort range (30000-32767) and is shared across all local
// deployments — only one agent can be reached this way at a time.
const LocalMessagingNodePort int32 = 30100

// LocalMessagingHost returns the host:port the Launch URL points at in
// local mode.
func LocalMessagingHost() string {
	return fmt.Sprintf("localhost:%d", LocalMessagingNodePort)
}

// LocalClient connects to a local Kubernetes cluster (Docker Desktop, kind, minikube)
// using a standard kubeconfig file. No AWS dependencies.
type LocalClient struct {
	clientset      *kubernetes.Clientset
	config         *rest.Config
	kubeconfigPath string
	kubeContext    string
	log            *logger.Logger
}

// NewLocalClient creates a new Kubernetes client from kubeconfig.
func NewLocalClient(cfg ClusterClientConfig) (*LocalClient, error) {
	log := cfg.Logger

	kubeconfigPath := cfg.KubeconfigPath
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory for default kubeconfig: %w", err)
		}
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	kubeContext := cfg.KubeContext
	if kubeContext == "" {
		kubeContext = os.Getenv("KUBE_CONTEXT")
	}
	// Empty kubeContext is fine — clientcmd uses current-context from kubeconfig

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Resolve the actual context name for logging
	rawConfig, _ := kubeConfig.RawConfig()
	resolvedContext := kubeContext
	if resolvedContext == "" {
		resolvedContext = rawConfig.CurrentContext
	}

	if log != nil {
		log.Info("Local K8s client initialized",
			"kubeconfig", kubeconfigPath,
			"context", resolvedContext,
			"server", restConfig.Host,
		)
	}

	return &LocalClient{
		clientset:      clientset,
		config:         restConfig,
		kubeconfigPath: kubeconfigPath,
		kubeContext:    resolvedContext,
		log:            log,
	}, nil
}

func (c *LocalClient) Clientset() *kubernetes.Clientset {
	return c.clientset
}

func (c *LocalClient) Config() *rest.Config {
	return c.config
}

func (c *LocalClient) CheckHealth() error {
	_, err := c.clientset.Discovery().ServerVersion()
	return err
}

func (c *LocalClient) GetServerVersion() (string, error) {
	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return version.GitVersion, nil
}

func (c *LocalClient) DiagnoseConnection() map[string]string {
	diag := map[string]string{
		"mode":       "local",
		"kubeconfig": c.kubeconfigPath,
		"context":    c.kubeContext,
	}

	if c.config != nil {
		diag["api_server"] = c.config.Host
	}

	if version, err := c.GetServerVersion(); err != nil {
		diag["connection"] = "failed: " + err.Error()
	} else {
		diag["connection"] = "ok"
		diag["server_version"] = version
	}

	return diag
}
