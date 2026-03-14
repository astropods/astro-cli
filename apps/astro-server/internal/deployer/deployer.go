package deployer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployer handles K8s apply and teardown operations for deployments.
type Deployer struct {
	K8sClient    k8s.ClusterClient
	AccountStore *account.AccountStore
	Cfg          *config.Config
	Store        *deploymentstore.Store
	Log          *logger.Logger
}

// Apply provisions K8s resources for a deployment using the current revision's spec.
func (d *Deployer) Apply(ctx context.Context, dep *deploymentstore.Deployment) (*k8s.ApplyResult, error) {
	// Load spec from current revision
	rev, err := d.Store.GetCurrentRevision(dep.ID)
	if err != nil {
		return nil, fmt.Errorf("get current revision: %w", err)
	}
	if rev == nil {
		return nil, fmt.Errorf("no current revision for deployment %s", dep.ID)
	}

	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal(rev.SpecJSON, &ds); err != nil {
		return nil, fmt.Errorf("unmarshal deployment spec: %w", err)
	}

	// Look up account name for namespace labels
	acct, err := d.AccountStore.GetByID(dep.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account %s: %w", dep.AccountID, err)
	}

	applier := k8s.NewApplier(d.K8sClient, k8s.ApplierConfig{
		Namespace:              dep.Namespace,
		RegistryURL:            d.Cfg.Deployment.RegistryURL,
		ProxyRegistryHost:      d.Cfg.Deployment.ProxyRegistryHost,
		Environment:            d.Cfg.Deployment.Environment,
		ImagePullPolicy:        imagePullPolicyForMode(d.Cfg.Deployment.K8sClientMode),
		IngressDomain:          d.Cfg.Deployment.IngressDomain,
		ACMCertificateARN:      d.Cfg.Deployment.ACMCertificateARN,
		ALBGroupName:           d.Cfg.Deployment.ALBGroupName,
		IngestionIngressDomain: d.Cfg.Deployment.IngestionIngressDomain,
		IngestionACMCertARN:    d.Cfg.Deployment.IngestionACMCertARN,
		IngestionALBGroupName:  d.Cfg.Deployment.IngestionALBGroupName,
		GalileoAPIKey:          d.Cfg.Deployment.GalileoAPIKey,
		GalileoProject:         d.Cfg.Deployment.GalileoProject,
		PodSubnetCIDRs:         d.Cfg.Deployment.PodSubnetCIDRs,
		NamespaceLabels: map[string]string{
			"astro.dev/account-id": dep.AccountID,
			"astro.dev/account":    acct.Name,
			"astro.dev/agent":      dep.AgentName,
			"astro.dev/build":      dep.BuildID,
		},
		NamespaceAnnotations: map[string]string{
			"astro.dev/display-name": dep.DisplayName,
		},
	})

	return applier.ApplyDeploymentSpec(ctx, &ds)
}

// Teardown deletes the K8s namespace for a deployment, cascading to all resources.
// Returns nil if the namespace is already gone (idempotent).
func (d *Deployer) Teardown(ctx context.Context, dep *deploymentstore.Deployment) error {
	err := d.K8sClient.Clientset().CoreV1().Namespaces().Delete(
		ctx, dep.Namespace, metav1.DeleteOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func imagePullPolicyForMode(mode string) corev1.PullPolicy {
	if mode == "local" {
		return corev1.PullNever
	}
	return corev1.PullAlways
}
